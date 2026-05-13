// Package client provides a WS-MAN (Web Services Management) HTTP client
// for Dell iDRAC 7. iDRAC 7 uses SOAP-over-HTTPS WS-MAN rather than Redfish.
//
// WS-MAN resource URIs are in the DCIM (Dell Common Information Model) namespace.
// Requests are HTTP POST to https://<host>/wsman with a SOAP envelope body.
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// -----------------------------------------------------------------------
// WS-MAN resource URIs (DCIM namespace for iDRAC 7)
// -----------------------------------------------------------------------

const (
	wsmanEndpoint = "/wsman"
	wsmanSchema   = "http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
	wsmanEnvNS    = "http://www.w3.org/2003/05/soap-envelope"
	wsmanAddrNS   = "http://schemas.xmlsoap.org/ws/2004/08/addressing"
	wsmanMgmtNS   = "http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
	cimNS         = "http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/"

	// DCIM resource base URI
	dcimBaseURI = "http://schemas.dell.com/wbem/wscim/1/cim-schema/2/"

	// ----------------------------------------------------------------
	// DCIM resource classes — Hardware inventory
	// ----------------------------------------------------------------
	ResourceSystemView     = dcimBaseURI + "DCIM_SystemView"
	ResourceCPUView        = dcimBaseURI + "DCIM_CPUView"
	ResourceMemoryView     = dcimBaseURI + "DCIM_MemoryView"
	ResourceNICView        = dcimBaseURI + "DCIM_NICView"
	ResourceControllerView = dcimBaseURI + "DCIM_ControllerView"
	ResourcePhysDiskView   = dcimBaseURI + "DCIM_PhysicalDiskView"
	ResourceVirtDiskView   = dcimBaseURI + "DCIM_VirtualDiskView"
	ResourceEnclosureView  = dcimBaseURI + "DCIM_EnclosureView"

	// ----------------------------------------------------------------
	// DCIM resource classes — Sensors & power
	// ----------------------------------------------------------------
	ResourceNumericSensor  = dcimBaseURI + "DCIM_NumericSensor"
	ResourcePSView         = dcimBaseURI + "DCIM_PowerSupplyView"
	ResourceFanView        = dcimBaseURI + "DCIM_Fan"
	ResourceBatteryView    = dcimBaseURI + "DCIM_Battery"

	// ----------------------------------------------------------------
	// DCIM resource classes — Front panel & removable media
	// ----------------------------------------------------------------
	ResourceFrontPanelView      = dcimBaseURI + "DCIM_FrontPanelMgmtControllerView"
	ResourceRemovableFlashMedia = dcimBaseURI + "DCIM_RemovableFlashMediaView"

	// ----------------------------------------------------------------
	// DCIM resource classes — Host OS network interfaces
	// ----------------------------------------------------------------
	ResourceHostNICView = dcimBaseURI + "DCIM_HostNetworkInterfaceView"

	// ----------------------------------------------------------------
	// DCIM resource classes — BIOS
	// ----------------------------------------------------------------
	ResourceBIOSEnum    = dcimBaseURI + "DCIM_BIOSEnumeration"
	ResourceBIOSString  = dcimBaseURI + "DCIM_BIOSString"
	ResourceBIOSInteger = dcimBaseURI + "DCIM_BIOSInteger"
	ResourceBIOSService = dcimBaseURI + "DCIM_BIOSService"

	// ----------------------------------------------------------------
	// DCIM resource classes — iDRAC card & service
	// ----------------------------------------------------------------
	ResourceiDRACCard    = dcimBaseURI + "DCIM_iDRACCardAttribute"
	ResourceiDRACService = dcimBaseURI + "DCIM_iDRACCardService"

	// ----------------------------------------------------------------
	// DCIM resource classes — Lifecycle / logs
	// ----------------------------------------------------------------
	ResourceLCLogEntry    = dcimBaseURI + "DCIM_LifecycleLogEntry"
	ResourceLCService     = dcimBaseURI + "DCIM_LCService"
	ResourceSELLogEntry   = dcimBaseURI + "DCIM_SELLogEntry"

	// ----------------------------------------------------------------
	// DCIM resource classes — Alerts / event filters
	// ----------------------------------------------------------------
	ResourceEventFilter   = dcimBaseURI + "DCIM_EventFilter"
	ResourceAlertPolicy   = dcimBaseURI + "DCIM_AlertDestination"

	// ----------------------------------------------------------------
	// DCIM resource classes — Storage management
	// ----------------------------------------------------------------
	ResourceRAIDService    = dcimBaseURI + "DCIM_RAIDService"
	ResourceStorageView    = dcimBaseURI + "DCIM_StorageView"

	// ----------------------------------------------------------------
	// DCIM resource classes — Firmware / Software
	// ----------------------------------------------------------------
	ResourceSoftwareIdentity  = dcimBaseURI + "DCIM_SoftwareIdentity"
	ResourceSoftwareInstSvc   = dcimBaseURI + "DCIM_SoftwareInstallationService"

	// ----------------------------------------------------------------
	// DCIM resource classes — Licenses
	// ----------------------------------------------------------------
	ResourceLicenseManageable = dcimBaseURI + "DCIM_LicenseManageable"
	ResourceLicenseMgmtSvc    = dcimBaseURI + "DCIM_LicenseManagementService"

	// ----------------------------------------------------------------
	// DCIM resource classes — Security / Sessions / Intrusion
	// ----------------------------------------------------------------
	ResourceSessionView   = dcimBaseURI + "DCIM_SessionView"
	ResourceIntrusionView = dcimBaseURI + "DCIM_PhysicalPackage"

	// ----------------------------------------------------------------
	// CIM (standard) resource classes
	// ----------------------------------------------------------------
	ResourceComputerSystem = "http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ComputerSystem"
)

// -----------------------------------------------------------------------
// Client
// -----------------------------------------------------------------------

// msgCounter generates unique MessageID values for WS-MAN requests.
var msgCounter atomic.Uint64

func nextMsgID(prefix string) string {
	return fmt.Sprintf("uuid:%s-%d", prefix, msgCounter.Add(1))
}
// New creates a new iDRAC 7 WS-MAN client.
// Set insecureSSL=true for self-signed certificates (common in labs).
func New(host, username, password string, insecureSSL bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSSL, //nolint:gosec // intentional for lab iDRAC
		},
	}
	return &Client{
		Host:     host,
		Username: username,
		Password: password,
		HTTPS:    true,
		http: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		cache: make(map[string]cacheEntry),
	}
}

// baseURL returns the WS-MAN endpoint URL.
func (c *Client) baseURL() string {
	scheme := "https"
	return fmt.Sprintf("%s://%s%s", scheme, c.Host, wsmanEndpoint)
}

// -----------------------------------------------------------------------
// SOAP envelope builder
// -----------------------------------------------------------------------

// envelope wraps a WS-MAN SOAP request.
type envelope struct {
	XMLName xml.Name `xml:"s:Envelope"`
	SAttr   string   `xml:"xmlns:s,attr"`
	WAttr   string   `xml:"xmlns:wsman,attr"`
	AAttr   string   `xml:"xmlns:a,attr"`
	NAttr   string   `xml:"xmlns:n,attr"`
	PAttr   string   `xml:"xmlns:p,attr"`
	Header  header   `xml:"s:Header"`
	Body    body     `xml:"s:Body"`
}

type header struct {
	To            string        `xml:"a:To"`
	ResourceURI   resourceURI   `xml:"wsman:ResourceURI"`
	ReplyTo       replyTo       `xml:"a:ReplyTo"`
	MessageID     string        `xml:"a:MessageID"`
	Action        action        `xml:"a:Action"`
	SelectorSet   *selectorSet  `xml:"wsman:SelectorSet,omitempty"`
	OperationTimeout string     `xml:"wsman:OperationTimeout,omitempty"`
}

type resourceURI struct {
	MustUnderstand string `xml:"s:mustUnderstand,attr"`
	Value          string `xml:",chardata"`
}

type replyTo struct {
	Address string `xml:"a:Address"`
}

type action struct {
	MustUnderstand string `xml:"s:mustUnderstand,attr"`
	Value          string `xml:",chardata"`
}

type selectorSet struct {
	Selectors []selector `xml:"wsman:Selector"`
}

type selector struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:",chardata"`
}

type body struct {
	Content interface{} `xml:",omitempty"`
}

// cacheEntry holds the result of a completed EnumerateAndPull for one resource URI.
type cacheEntry struct {
	items []RawItem
	err   error
}

// Client holds connection details and an HTTP client for talking to iDRAC 7.
type Client struct {
	Host     string
	Username string
	Password string
	HTTPS    bool
	http     *http.Client
	mu       sync.Mutex            // iDRAC 7 cannot handle concurrent WS-MAN requests
	cache    map[string]cacheEntry // keyed by resource URI; populated on first fetch
	cacheMu  sync.Mutex            // guards cache map
}

// pullBody is used to pull results from an enumeration context.
type pullBody struct {
	XMLName            xml.Name `xml:"wsen:Pull"`
	WsenAttr           string   `xml:"xmlns:wsen,attr"`
	EnumerationContext string   `xml:"wsen:EnumerationContext"`
	MaxElements        int      `xml:"wsen:MaxElements"`
}

// -----------------------------------------------------------------------
// Response types
// -----------------------------------------------------------------------

// EnumerateResponse holds the enumeration context returned by Enumerate.
type EnumerateResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		EnumerateResponse struct {
			EnumerationContext string `xml:"EnumerationContext"`
		} `xml:"EnumerateResponse"`
	} `xml:"Body"`
}

// PullResponse holds raw XML items returned by Pull.
type PullResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		PullResponse struct {
			Items []RawItem `xml:"Items>*"`
			EndOfSequence *struct{} `xml:"EndOfSequence"`
		} `xml:"PullResponse"`
	} `xml:"Body"`
}

// RawItem holds a single instance returned from an enumerate+pull.
type RawItem struct {
	XMLName xml.Name
	Fields  []xml.Token
	Raw     []byte
}

func (r *RawItem) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	r.XMLName = start.Name
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	_ = enc.EncodeToken(start)
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		if end, ok := tok.(xml.EndElement); ok && end.Name == start.Name {
			_ = enc.EncodeToken(tok)
			break
		}
		_ = enc.EncodeToken(xml.CopyToken(tok))
	}
	_ = enc.Flush()
	r.Raw = buf.Bytes()
	return nil
}

// GetResponse holds the response from a Get request.
type GetResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Inner []byte `xml:",innerxml"`
	} `xml:"Body"`
}

// InvokeResponse holds the return value from an Invoke request.
type InvokeResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Inner []byte `xml:",innerxml"`
	} `xml:"Body"`
}

// -----------------------------------------------------------------------
// Core request methods
// -----------------------------------------------------------------------

// PostRaw sends a pre-built SOAP envelope directly to the WS-MAN endpoint.
// This is used by resources that construct their own envelopes.
func (c *Client) PostRaw(envelope string) ([]byte, error) {
	return c.post("", envelope)
}

func (c *Client) post(soapAction, body string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req, err := http.NewRequest(http.MethodPost, c.baseURL(), bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Content-Type", "application/soap+xml;charset=UTF-8")
	if soapAction != "" {
		req.Header.Set("SOAPAction", soapAction)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request to %s: %w", c.baseURL(), err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from iDRAC: %s", resp.StatusCode, string(data))
	}

	// Detect SOAP faults — iDRAC returns HTTP 200 even for fault responses.
	// A fault body contains a <s:Fault> element; parse it and return an error
	// immediately so callers don't hang trying to parse a missing context.
	if faultMsg := parseSoapFault(data); faultMsg != "" {
		return nil, fmt.Errorf("SOAP fault: %s", faultMsg)
	}

	return data, nil
}

// parseSoapFault scans raw SOAP XML for a <s:Fault> element and returns a
// human-readable description, or "" if no fault is present.
func parseSoapFault(data []byte) string {
	var env struct {
		Body struct {
			Fault struct {
				Code struct {
					Value   string `xml:"Value"`
					Subcode struct {
						Value string `xml:"Value"`
					} `xml:"Subcode"`
				} `xml:"Code"`
				Reason struct {
					Text string `xml:"Text"`
				} `xml:"Reason"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return ""
	}
	f := env.Body.Fault
	if f.Code.Value == "" {
		return ""
	}
	msg := f.Code.Value
	if f.Code.Subcode.Value != "" {
		msg += "/" + f.Code.Subcode.Value
	}
	if f.Reason.Text != "" {
		msg += ": " + f.Reason.Text
	}
	return msg
}

// buildEnumerate creates a WS-MAN Enumerate SOAP envelope for a resource class.
func buildEnumerate(resourceURI, host string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:wsen="http://schemas.xmlsoap.org/ws/2004/09/enumeration">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">%s</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/09/enumeration/Enumerate</a:Action>
    <a:MessageID>%s</a:MessageID>
    <wsman:OperationTimeout>PT15S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <wsen:Enumerate/>
  </s:Body>
</s:Envelope>`, host, resourceURI, nextMsgID("enum"))
}

// buildPull creates a WS-MAN Pull SOAP envelope to retrieve instances.
func buildPull(resourceURI, host, enumCtx string, maxElements int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:wsen="http://schemas.xmlsoap.org/ws/2004/09/enumeration">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">%s</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/09/enumeration/Pull</a:Action>
    <a:MessageID>%s</a:MessageID>
    <wsman:OperationTimeout>PT15S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <wsen:Pull>
      <wsen:EnumerationContext>%s</wsen:EnumerationContext>
      <wsen:MaxElements>%d</wsen:MaxElements>
    </wsen:Pull>
  </s:Body>
</s:Envelope>`, host, resourceURI, nextMsgID("pull"), enumCtx, maxElements)
}

// buildGet creates a WS-MAN Get SOAP envelope for a single instance.
func buildGet(resourceURI, host string, selectors map[string]string) string {
	selXML := ""
	if len(selectors) > 0 {
		selXML = "<wsman:SelectorSet>"
		for k, v := range selectors {
			selXML += fmt.Sprintf(`<wsman:Selector Name="%s">%s</wsman:Selector>`, k, v)
		}
		selXML += "</wsman:SelectorSet>"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">%s</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/transfer/Get</a:Action>
    <a:MessageID>%s</a:MessageID>
    <wsman:OperationTimeout>PT15S</wsman:OperationTimeout>
    %s
  </s:Header>
  <s:Body/>
</s:Envelope>`, host, resourceURI, nextMsgID("get"), selXML)
}

// -----------------------------------------------------------------------
// High-level API methods
// -----------------------------------------------------------------------

// EnumerateAndPull performs a WS-MAN Enumerate + Pull sequence and returns
// all raw XML items for the given resource class URI.
// Results are cached: subsequent calls for the same URI return instantly from memory.
func (c *Client) EnumerateAndPull(resourceClass string) ([]RawItem, error) {
	// Fast path: return from cache if already fetched.
	c.cacheMu.Lock()
	if entry, ok := c.cache[resourceClass]; ok {
		c.cacheMu.Unlock()
		return entry.items, entry.err
	}
	c.cacheMu.Unlock()

	// Slow path: fetch from iDRAC (serialised by mu).
	items, err := c.enumerateAndPull(resourceClass)

	// Store in cache regardless of error so we don't retry on failure.
	c.cacheMu.Lock()
	c.cache[resourceClass] = cacheEntry{items: items, err: err}
	c.cacheMu.Unlock()

	return items, err
}

func (c *Client) enumerateAndPull(resourceClass string) ([]RawItem, error) {
	// Step 1: Enumerate
	enumEnv := buildEnumerate(resourceClass, c.Host)
	enumData, err := c.post("", enumEnv)
	if err != nil {
		return nil, fmt.Errorf("enumerate %s: %w", resourceClass, err)
	}

	// Parse EnumerationContext from Enumerate response using token streaming
	// (xml.Unmarshal path matching fails with wsen: namespace prefix)
	ctx := extractEnumerationContext(enumData)
	if ctx == "" {
		return nil, fmt.Errorf("enumerate %s: empty EnumerationContext in response (resource may be unsupported on this firmware)", resourceClass)
	}

	// Step 2: Pull all pages
	var allItems []RawItem
	for {
		pullEnv := buildPull(resourceClass, c.Host, ctx, 100)
		pullData, err := c.post("", pullEnv)
		if err != nil {
			return nil, fmt.Errorf("pull %s: %w", resourceClass, err)
		}

		items, done, nextCtx, err := parsePullResponse(pullData)
		if err != nil {
			return nil, fmt.Errorf("parsing pull response for %s: %w", resourceClass, err)
		}
		allItems = append(allItems, items...)
		if done {
			break
		}
		ctx = nextCtx
	}
	return allItems, nil
}

// parsePullResponse streams a WS-MAN Pull response and extracts items using
// a token-based approach that is robust to XML namespace prefixes.
// Go's xml.Unmarshal with path "Items>*" fails when elements use namespace
// prefixes (e.g. wsen:Items, n1:DCIM_SystemView), so we parse token by token.
func parsePullResponse(data []byte) (items []RawItem, done bool, nextCtx string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var inItems, inItem bool
	var depth int
	var buf bytes.Buffer
	var currentName xml.Name

	for {
		var tok xml.Token
		tok, err = dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				err = nil
			}
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Items":
				inItems = true
			case "EndOfSequence":
				done = true
			case "EnumerationContext":
				if !inItems {
					// continuation context for next Pull page
					var s string
					if e := dec.DecodeElement(&s, &t); e == nil {
						nextCtx = s
					}
				}
			default:
				if inItems && !inItem {
					// top-level item element
					inItem = true
					depth = 1
					currentName = t.Name
					buf.Reset()
					enc := xml.NewEncoder(&buf)
					_ = enc.EncodeToken(t)
					_ = enc.Flush()
				} else if inItem {
					depth++
					enc := xml.NewEncoder(&buf)
					_ = enc.EncodeToken(t)
					_ = enc.Flush()
				}
			}
		case xml.EndElement:
			if inItem {
				enc := xml.NewEncoder(&buf)
				_ = enc.EncodeToken(t)
				_ = enc.Flush()
				depth--
				if depth == 0 {
					items = append(items, RawItem{
						XMLName: currentName,
						Raw:     append([]byte(nil), buf.Bytes()...),
					})
					inItem = false
				}
			} else if t.Name.Local == "Items" {
				inItems = false
			}
		case xml.CharData:
			if inItem {
				enc := xml.NewEncoder(&buf)
				_ = enc.EncodeToken(t)
				_ = enc.Flush()
			}
		}
	}
}

// Get performs a WS-MAN Get for a single instance.
func (c *Client) Get(resourceClass string, selectors map[string]string) ([]byte, error) {
	getEnv := buildGet(resourceClass, c.Host, selectors)
	data, err := c.post("", getEnv)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", resourceClass, err)
	}
	return data, nil
}

// Invoke performs a WS-MAN Invoke (method call) on a resource.
func (c *Client) Invoke(resourceClass, method, host string, selectors map[string]string, inputXML string) ([]byte, error) {
	selXML := ""
	if len(selectors) > 0 {
		selXML = "<wsman:SelectorSet>"
		for k, v := range selectors {
			selXML += fmt.Sprintf(`<wsman:Selector Name="%s">%s</wsman:Selector>`, k, v)
		}
		selXML += "</wsman:SelectorSet>"
	}

	actionURI := fmt.Sprintf("%s/%s", resourceClass, method)
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="%s">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">%s</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">%s</a:Action>
    <a:MessageID>%s</a:MessageID>
    <wsman:OperationTimeout>PT300S</wsman:OperationTimeout>
    %s
  </s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, resourceClass, host, resourceClass, actionURI, nextMsgID("invoke"), selXML, inputXML)

	return c.post(actionURI, envelope)
}

// -----------------------------------------------------------------------
// XML parsing helpers
// -----------------------------------------------------------------------

// extractEnumerationContext finds the EnumerationContext value in a WS-MAN
// Enumerate response using token streaming (namespace-safe).
func extractEnumerationContext(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "EnumerationContext" {
			var s string
			if err := dec.DecodeElement(&s, &se); err == nil {
				return s
			}
		}
	}
}

// FieldValue extracts the text value of a named XML element from raw XML bytes.
func FieldValue(raw []byte, localName string) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var capture bool
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == localName {
				capture = true
			}
		case xml.CharData:
			if capture {
				return string(bytes.TrimSpace(t))
			}
		case xml.EndElement:
			capture = false
		}
	}
	return ""
}

// AllFieldValues extracts all text values for a named XML element (handles repeated elements).
func AllFieldValues(raw []byte, localName string) []string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var results []string
	var capture bool
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == localName {
				capture = true
			}
		case xml.CharData:
			if capture {
				results = append(results, string(bytes.TrimSpace(t)))
			}
		case xml.EndElement:
			if t.Name.Local == localName {
				capture = false
			}
		}
	}
	return results
}
