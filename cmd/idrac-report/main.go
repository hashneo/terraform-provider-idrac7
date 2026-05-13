// cmd/idrac-report — standalone iDRAC 7 inventory report tool.
//
// Usage:
//
//	go run ./cmd/idrac-report [flags]
//
// Flags (each falls back to the matching env var):
//
//	--host       IDRAC_HOST      iDRAC hostname or IP (e.g. 192.168.1.30)
//	--user       IDRAC_USER      username (default: root)
//	--password   IDRAC_PASSWORD  password
//	--out        idrac-report.html  output file (.json → JSON, else HTML)
//	--insecure                   skip TLS verification (default: true)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
)

// ── CLI helpers ───────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Section model ─────────────────────────────────────────────────────────────

type section struct {
	Key       string
	Title     string
	Group     string
	Data      any
	Count     int
	Err       error
	Elapsed   time.Duration
	Collapsed bool
}

// ── Serial fetch (iDRAC 7 cannot handle concurrent WS-MAN calls) ──────────────

type fetcher struct {
	key       string
	title     string
	group     string
	collapsed bool
	fn        func() (any, int, error)
}

func runFetchers(fetchers []fetcher) []section {
	results := make([]section, len(fetchers))
	for i, f := range fetchers {
		start := time.Now()
		data, count, err := f.fn()
		results[i] = section{
			Key:       f.key,
			Title:     f.title,
			Group:     f.group,
			Data:      data,
			Count:     count,
			Err:       err,
			Elapsed:   time.Since(start),
			Collapsed: f.collapsed,
		}
		if err != nil {
			fmt.Printf("  %-28s ⚠  %s\n", f.key, err)
		} else {
			fmt.Printf("  %-28s %d items  (%s)\n", f.key, count, fmtElapsed(time.Since(start)-time.Since(start)+results[i].Elapsed))
		}
	}
	return results
}

// enumerateFetcher wraps a DCIM enumerate into a fetcher fn returning []rawRow.
type rawRow struct {
	fields map[string]string
}

// MarshalJSON serialises rawRow as a plain JSON object so that writeJSON
// produces {"field": "value", ...} instead of {}.
func (r rawRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.fields)
}

func makeEnumFetcher(c *client.Client, resourceURI string, fieldNames []string) func() (any, int, error) {
	return func() (any, int, error) {
		items, err := c.EnumerateAndPull(resourceURI)
		if err != nil {
			return nil, 0, err
		}
		rows := make([]rawRow, 0, len(items))
		for _, item := range items {
			row := rawRow{fields: make(map[string]string, len(fieldNames))}
			for _, f := range fieldNames {
				row.fields[f] = client.FieldValue(item.Raw, f)
			}
			rows = append(rows, row)
		}
		return rows, len(rows), nil
	}
}

// ── JSON output ───────────────────────────────────────────────────────────────

func writeJSON(sections []section, outPath string) error {
	obj := map[string]any{
		"generated": time.Now().UTC().Format(time.RFC3339),
	}
	data := map[string]any{}
	for _, s := range sections {
		if s.Err != nil {
			data[s.Key] = map[string]string{"error": s.Err.Error()}
		} else {
			data[s.Key] = s.Data
		}
	}
	obj["sections"] = data
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, b, 0o644)
}

// ── HTML template ─────────────────────────────────────────────────────────────

const htmlTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>iDRAC 7 Inventory Report</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;font-size:13px;background:#f5f6fa;color:#222}
header{background:#1a1a2e;color:#fff;padding:16px 24px;display:flex;align-items:center;gap:24px;flex-wrap:wrap}
header h1{font-size:18px;font-weight:600;letter-spacing:.5px}
.header-meta{font-size:12px;color:#aab}
.header-counts{display:flex;gap:12px;margin-left:auto}
.hcount{background:#ffffff22;border-radius:12px;padding:3px 10px;font-size:12px}
.layout{display:flex;min-height:calc(100vh - 56px)}
nav{width:200px;min-width:200px;background:#fff;border-right:1px solid #e2e8f0;padding:16px 0;position:sticky;top:0;height:calc(100vh - 56px);overflow-y:auto}
nav .group-label{padding:10px 16px 4px;font-size:10px;font-weight:700;text-transform:uppercase;color:#94a3b8;letter-spacing:.8px}
nav a{display:block;padding:6px 16px 6px 20px;color:#475569;text-decoration:none;font-size:12px;border-left:2px solid transparent;transition:all .15s}
nav a:hover{background:#f1f5f9;color:#1a1a2e;border-left-color:#6366f1}
main{flex:1;padding:20px 24px;min-width:0}
.section-card{background:#fff;border:1px solid #e2e8f0;border-radius:8px;margin-bottom:16px;overflow:hidden}
details>summary{list-style:none;padding:12px 16px;cursor:pointer;display:flex;align-items:center;gap:8px;user-select:none;background:#fff;border-bottom:1px solid transparent}
details>summary::-webkit-details-marker{display:none}
details[open]>summary{border-bottom-color:#e2e8f0;background:#f8fafc}
summary::before{content:"▶";font-size:10px;color:#94a3b8;transition:transform .2s;display:inline-block}
details[open]>summary::before{transform:rotate(90deg)}
.section-title{font-weight:600;font-size:13px;color:#1e293b}
.section-count{background:#e2e8f0;color:#64748b;border-radius:10px;padding:1px 8px;font-size:11px}
.section-count.error{background:#fee2e2;color:#dc2626}
.elapsed{margin-left:auto;font-size:11px;color:#94a3b8}
.group-badge{font-size:10px;padding:1px 7px;border-radius:8px;font-weight:500}
.badge-hw{background:#ede9fe;color:#7c3aed}
.badge-storage{background:#dbeafe;color:#2563eb}
.badge-fw{background:#fef9c3;color:#854d0e}
.badge-bios{background:#d1fae5;color:#065f46}
.badge-mgmt{background:#fee2e2;color:#9f1239}
.error-banner{padding:12px 16px;background:#fff7ed;border-left:3px solid #f97316;color:#9a3412;font-size:12px}
.table-wrap{overflow-x:auto;max-height:500px;overflow-y:auto}
table{width:100%;border-collapse:collapse;font-size:12px}
thead th{position:sticky;top:0;background:#f8fafc;padding:8px 12px;text-align:left;font-weight:600;color:#64748b;border-bottom:2px solid #e2e8f0;white-space:nowrap}
tbody tr:nth-child(even){background:#f9fafb}
tbody tr:hover{background:#f1f5f9}
td{padding:6px 12px;color:#334155;vertical-align:top;border-bottom:1px solid #f1f5f9;max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
td.wrap{white-space:normal;word-break:break-word}
td.mono{font-family:ui-monospace,monospace;font-size:11px}
.badge{display:inline-block;border-radius:10px;padding:1px 8px;font-size:11px;font-weight:500}
.green{background:#d1fae5;color:#065f46}
.red{background:#fee2e2;color:#991b1b}
.amber{background:#fef3c7;color:#92400e}
.grey{background:#f1f5f9;color:#64748b}
.kv-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:8px;padding:16px}
.kv-item{background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:10px 12px}
.kv-label{font-size:10px;font-weight:600;text-transform:uppercase;color:#94a3b8;margin-bottom:2px}
.kv-value{font-size:13px;color:#1e293b;word-break:break-word}
</style>
</head>
<body>
<header>
  <h1>&#x1F5A5; iDRAC 7 Inventory Report</h1>
  <span class="header-meta">Generated: {{.Generated}}</span>
  {{if .Host}}<span class="header-meta">Host: <code>{{.Host}}</code></span>{{end}}
  <div class="header-counts">
    {{range .GroupCounts}}<span class="hcount">{{.Name}}: {{.Total}} items</span>{{end}}
  </div>
</header>
<div class="layout">
<nav>
  {{range .Groups}}
  <div class="group-label">{{.Name}}</div>
  {{range .Sections}}
  <a href="#{{.Key}}">{{.Title}}{{if .Err}} ⚠{{else if gt .Count 0}} ({{.Count}}){{end}}</a>
  {{end}}
  {{end}}
</nav>
<main>
{{range .Sections}}
<div class="section-card" id="{{.Key}}">
  <details{{if not .Collapsed}} open{{end}}>
    <summary>
      <span class="section-title">{{.Title}}</span>
      {{if .Err}}
        <span class="section-count error">error</span>
      {{else}}
        <span class="section-count">{{.Count}}</span>
      {{end}}
      <span class="group-badge {{.GroupClass}}">{{.Group}}</span>
      <span class="elapsed">{{.ElapsedStr}}</span>
    </summary>
    {{if .Err}}
      <div class="error-banner">&#9888; API error: {{.Err}}</div>
    {{else}}
      {{.TableHTML}}
    {{end}}
  </details>
</div>
{{end}}
</main>
</div>
</body>
</html>`

// ── Template data types ───────────────────────────────────────────────────────

type templateData struct {
	Generated   string
	Host        string
	GroupCounts []groupCount
	Groups      []groupNav
	Sections    []sectionView
}

type groupCount struct {
	Name  string
	Total int
}

type groupNav struct {
	Name     string
	Sections []sectionView
}

type sectionView struct {
	Key        string
	Title      string
	Group      string
	GroupClass string
	Count      int
	Err        error
	ElapsedStr string
	Collapsed  bool
	TableHTML  template.HTML
}

func groupClass(group string) string {
	switch group {
	case "Hardware":
		return "badge-hw"
	case "Storage":
		return "badge-storage"
	case "Firmware":
		return "badge-fw"
	case "BIOS":
		return "badge-bios"
	case "Management":
		return "badge-mgmt"
	}
	return "grey"
}

func fmtElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// ── HTML helpers ──────────────────────────────────────────────────────────────

func str(s string) template.HTML {
	if s == "" {
		return `<span style="color:#cbd5e1">—</span>`
	}
	return template.HTML(template.HTMLEscapeString(s))
}

func statusBadge(s string) template.HTML {
	// PrimaryStatus: 0=Unknown,1=OK,2=Degraded/Warning,3=Error/Critical
	switch s {
	case "0":
		return `<span class="badge grey">Unknown</span>`
	case "1", "OK":
		return `<span class="badge green">OK</span>`
	case "2", "Degraded", "Warning":
		return `<span class="badge amber">Warning</span>`
	case "3", "Error", "Critical":
		return `<span class="badge red">Critical</span>`
	}
	if s == "" {
		return `<span style="color:#cbd5e1">—</span>`
	}
	return template.HTML(`<span class="badge grey">` + template.HTMLEscapeString(s) + `</span>`)
}

func openTable() string {
	return `<div class="table-wrap"><table><thead><tr>`
}

func th(cols ...string) string {
	var sb strings.Builder
	for _, c := range cols {
		sb.WriteString(`<th>` + template.HTMLEscapeString(c) + `</th>`)
	}
	sb.WriteString(`</tr></thead><tbody>`)
	return sb.String()
}

func closeTable() string { return `</tbody></table></div>` }

func td(vals ...template.HTML) string {
	var sb strings.Builder
	sb.WriteString("<tr>")
	for _, v := range vals {
		sb.WriteString(`<td>` + string(v) + `</td>`)
	}
	sb.WriteString("</tr>")
	return sb.String()
}

func tdc(class string, val template.HTML) string {
	return `<td class="` + class + `">` + string(val) + `</td>`
}

// ── Generic table builder ─────────────────────────────────────────────────────

// tableSpec describes how to render a []rawRow as an HTML table.
type tableSpec struct {
	headers []string
	fields  []string // parallel to headers; special prefix "status:" → statusBadge
	mono    map[int]bool
	wrap    map[int]bool
}

func buildGenericTable(rows []rawRow, spec tableSpec) template.HTML {
	if len(rows) == 0 {
		return `<div class="error-banner" style="background:#f8fafc;border-color:#e2e8f0;color:#64748b">No items returned.</div>`
	}
	var sb strings.Builder
	sb.WriteString(openTable())
	sb.WriteString(th(spec.headers...))
	for _, row := range rows {
		sb.WriteString("<tr>")
		for i, f := range spec.fields {
			val := row.fields[f]
			class := ""
			if spec.mono[i] {
				class = "mono"
			} else if spec.wrap[i] {
				class = "wrap"
			}
			var cell template.HTML
			if strings.HasPrefix(f, "status:") {
				cell = statusBadge(val)
			} else {
				cell = str(val)
			}
			if class != "" {
				sb.WriteString(tdc(class, cell))
			} else {
				sb.WriteString(`<td>` + string(cell) + `</td>`)
			}
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString(closeTable())
	return template.HTML(sb.String())
}

// kvCard renders a single-item []rawRow as a grid of key/value cards.
func kvCard(rows []rawRow, fields []string, labels []string) template.HTML {
	if len(rows) == 0 {
		return `<div class="error-banner">No data returned.</div>`
	}
	row := rows[0]
	var sb strings.Builder
	sb.WriteString(`<div class="kv-grid">`)
	for i, f := range fields {
		label := f
		if i < len(labels) {
			label = labels[i]
		}
		val := row.fields[f]
		if val == "" {
			val = "—"
		}
		sb.WriteString(`<div class="kv-item"><div class="kv-label">` +
			template.HTMLEscapeString(label) +
			`</div><div class="kv-value">` +
			template.HTMLEscapeString(val) +
			`</div></div>`)
	}
	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

// ── Section → HTML ────────────────────────────────────────────────────────────

// sectionSpecs maps section key → how to render its []rawRow.
type renderFn func(rows []rawRow) template.HTML

var renderers map[string]renderFn

func init() {
	renderers = map[string]renderFn{
		"system": func(rows []rawRow) template.HTML {
			return kvCard(rows,
				[]string{"ServiceTag", "Model", "Manufacturer", "BIOSVersionString",
					"LifecycleControllerVersion", "PowerState", "HostName", "OSName",
					"SysMemTotalSize", "CPUSocketsPopulated"},
				[]string{"Service Tag", "Model", "Manufacturer", "BIOS Version",
					"iDRAC Firmware", "Power State", "Hostname", "OS Name",
					"Total RAM (MB)", "CPU Sockets"},
			)
		},
		"cpus": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Manufacturer", "Speed (MHz)", "Cores", "Threads", "Status"},
				fields:  []string{"FQDD", "Model", "Manufacturer", "MaxClockSpeed", "NumberOfProcessorCores", "NumberOfEnabledThreads", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"memory": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Manufacturer", "Part Number", "Size (MB)", "Speed (MHz)", "Rank", "Status"},
				fields:  []string{"FQDD", "Manufacturer", "PartNumber", "Size", "Speed", "Rank", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"nics": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Product", "MAC", "Link Speed", "Status"},
				fields:  []string{"FQDD", "ProductName", "PermanentMACAddress", "LinkSpeed", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true, 2: true},
			})
		},
		"controllers": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Product", "Firmware", "Cache Size (MB)", "Status"},
				fields:  []string{"FQDD", "ProductName", "ControllerFirmwareVersion", "CacheSizeInMB", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"physical_disks": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Manufacturer", "Model", "Serial", "Size (B)", "Media", "Bus", "Slot", "Status"},
				fields:  []string{"FQDD", "Manufacturer", "Model", "SerialNumber", "SizeInBytes", "MediaType", "BusProtocol", "Slot", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true, 3: true},
			})
		},
		"virtual_disks": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "RAID", "Size (B)", "Read Cache", "Write Cache", "RAID Status", "Status"},
				fields:  []string{"FQDD", "Name", "RAIDTypes", "SizeInBytes", "ReadCachePolicy", "WriteCachePolicy", "RAIDStatus", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"enclosures": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Product", "Service Tag", "Slots", "Firmware", "Status"},
				fields:  []string{"FQDD", "Name", "ProductName", "ServiceTag", "SlotCount", "FirmwareVersion", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"fans": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Speed (RPM)", "Lower Critical (RPM)", "State", "Status"},
				fields:  []string{"FQDD", "ElementName", "CurrentReading", "LowerThresholdCritical", "EnabledState", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"psus": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Input Watts", "Max Output Watts", "Firmware", "Status"},
				fields:  []string{"FQDD", "ProductName", "InputWatts", "TotalOutputPower", "FirmwareVersion", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"batteries": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Type", "Charge State", "Predicted Capacity", "Status"},
				fields:  []string{"FQDD", "Name", "Type", "ChargeState", "PredictedCapacity", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"sensors": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Type", "Reading", "Units", "Upper Critical", "Lower Critical", "Status"},
				fields:  []string{"FQDD", "ElementName", "SensorType", "CurrentReading", "UnitModifier", "UpperThresholdCritical", "LowerThresholdCritical", "status:HealthState"},
				mono:    map[int]bool{0: true},
			})
		},
		"firmware": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Component", "Version", "Update Required", "Status"},
				fields:  []string{"FQDD", "ElementName", "VersionString", "UpdateRequired", "status:Status"},
				mono:    map[int]bool{0: true, 2: true},
			})
		},
		"front_panel": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Firmware", "Last Update", "Status"},
				fields:  []string{"FQDD", "FirmwareVersion", "LastUpdateTime", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"flash_media": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Name", "Size (MB)", "Write Protected", "Last Update", "Status"},
				fields:  []string{"FQDD", "Name", "Size", "WriteProtected", "LastUpdateTime", "status:PrimaryStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"licenses": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"Entitlement ID", "Description", "Type", "Start", "End", "Device Count", "Status"},
				fields:  []string{"EntitlementID", "LicenseDescription", "LicenseType", "LicenseStartDate", "LicenseEndDate", "AllowedDeviceCount", "status:PrimaryStatus"},
				wrap:    map[int]bool{1: true},
			})
		},
		"sessions": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"Session ID", "Username", "IP", "Type", "Started"},
				fields:  []string{"SessionID", "UserName", "IPAddress", "SessionType", "StartTime"},
				mono:    map[int]bool{2: true},
			})
		},
		"intrusion": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"FQDD", "Type", "Status"},
				fields:  []string{"FQDD", "IntrusionType", "IntrusionStatus"},
				mono:    map[int]bool{0: true},
			})
		},
		"lc_logs": func(rows []rawRow) template.HTML {
			return buildGenericTable(rows, tableSpec{
				headers: []string{"Record ID", "Timestamp", "Severity", "Category", "Agent", "Msg ID", "Message"},
				fields:  []string{"RecordID", "CreationTimeStamp", "Severity", "Category", "AgentID", "MessageID", "Message"},
				wrap:    map[int]bool{6: true},
			})
		},
	}
}

func buildTableHTML(s section) template.HTML {
	if s.Err != nil {
		return ""
	}
	rows, ok := s.Data.([]rawRow)
	if !ok {
		return `<div class="error-banner">unknown data type</div>`
	}
	if fn, ok := renderers[s.Key]; ok {
		return fn(rows)
	}
	// fallback: generic key→value table for unknown sections
	if len(rows) == 0 {
		return `<div class="error-banner" style="background:#f8fafc;border-color:#e2e8f0;color:#64748b">No items returned.</div>`
	}
	// collect keys from first row
	keys := make([]string, 0, len(rows[0].fields))
	for k := range rows[0].fields {
		keys = append(keys, k)
	}
	return buildGenericTable(rows, tableSpec{headers: keys, fields: keys})
}

// ── Render HTML ───────────────────────────────────────────────────────────────

func writeHTML(sections []section, host, outPath string) error {
	views := make([]sectionView, len(sections))
	for i, s := range sections {
		views[i] = sectionView{
			Key:        s.Key,
			Title:      s.Title,
			Group:      s.Group,
			GroupClass: groupClass(s.Group),
			Count:      s.Count,
			Err:        s.Err,
			ElapsedStr: fmtElapsed(s.Elapsed),
			Collapsed:  s.Collapsed,
			TableHTML:  buildTableHTML(s),
		}
	}

	groupOrder := []string{"Hardware", "Storage", "Firmware", "BIOS", "Management"}
	groupMap := map[string][]sectionView{}
	for _, v := range views {
		groupMap[v.Group] = append(groupMap[v.Group], v)
	}
	var groups []groupNav
	groupCounts := make([]groupCount, 0)
	for _, g := range groupOrder {
		secs := groupMap[g]
		if len(secs) == 0 {
			continue
		}
		total := 0
		for _, s := range secs {
			total += s.Count
		}
		groups = append(groups, groupNav{Name: g, Sections: secs})
		groupCounts = append(groupCounts, groupCount{Name: g, Total: total})
	}

	data := templateData{
		Generated:   time.Now().Format("2006-01-02 15:04:05 MST"),
		Host:        host,
		GroupCounts: groupCounts,
		Groups:      groups,
		Sections:    views,
	}

	funcMap := template.FuncMap{
		"not": func(b bool) bool { return !b },
	}
	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTmpl)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	host := flag.String("host", envOr("IDRAC_HOST", ""), "iDRAC hostname or IP (env: IDRAC_HOST)")
	user := flag.String("user", envOr("IDRAC_USER", "root"), "Username (env: IDRAC_USER)")
	password := flag.String("password", envOr("IDRAC_PASSWORD", ""), "Password (env: IDRAC_PASSWORD)")
	out := flag.String("out", "idrac-report.html", "Output file (.json → JSON, else HTML)")
	insecure := flag.Bool("insecure", true, "Skip TLS verification (default: true)")
	flag.Parse()

	if *host == "" {
		fmt.Fprintln(os.Stderr, "error: --host is required (or set IDRAC_HOST)")
		os.Exit(1)
	}
	if *password == "" {
		fmt.Fprintln(os.Stderr, "error: --password is required (or set IDRAC_PASSWORD)")
		os.Exit(1)
	}

	format := "html"
	if strings.ToLower(filepath.Ext(*out)) == ".json" {
		format = "json"
	}

	c := client.New(*host, *user, *password, *insecure)

	// Field sets for each DCIM resource
	systemFields := []string{"ServiceTag", "Model", "Manufacturer", "BIOSVersionString",
		"LifecycleControllerVersion", "PowerState", "HostName", "OSName",
		"SysMemTotalSize", "CPUSocketsPopulated"}

	cpuFields := []string{"FQDD", "Model", "Manufacturer", "MaxClockSpeed",
		"NumberOfProcessorCores", "NumberOfEnabledThreads", "PrimaryStatus"}

	memFields := []string{"FQDD", "Manufacturer", "PartNumber", "Size", "Speed", "Rank", "PrimaryStatus"}

	nicFields := []string{"FQDD", "ProductName", "PermanentMACAddress", "LinkSpeed", "PrimaryStatus"}

	ctrlFields := []string{"FQDD", "ProductName", "ControllerFirmwareVersion", "CacheSizeInMB", "PrimaryStatus"}

	pdFields := []string{"FQDD", "Manufacturer", "Model", "SerialNumber", "SizeInBytes",
		"MediaType", "BusProtocol", "Slot", "PrimaryStatus"}

	vdFields := []string{"FQDD", "Name", "RAIDTypes", "SizeInBytes", "ReadCachePolicy",
		"WriteCachePolicy", "RAIDStatus", "PrimaryStatus"}

	encFields := []string{"FQDD", "Name", "ProductName", "ServiceTag", "SlotCount",
		"FirmwareVersion", "PrimaryStatus"}

	fanFields := []string{"FQDD", "ElementName", "CurrentReading", "LowerThresholdCritical",
		"EnabledState", "PrimaryStatus"}

	psuFields := []string{"FQDD", "ProductName", "InputWatts", "TotalOutputPower",
		"FirmwareVersion", "PrimaryStatus"}

	battFields := []string{"FQDD", "Name", "Type", "ChargeState", "PredictedCapacity", "PrimaryStatus"}

	sensorFields := []string{"FQDD", "ElementName", "SensorType", "CurrentReading",
		"UnitModifier", "UpperThresholdCritical", "LowerThresholdCritical", "HealthState"}

	fwFields := []string{"FQDD", "ElementName", "VersionString", "UpdateRequired", "Status"}

	fpFields := []string{"FQDD", "FirmwareVersion", "LastUpdateTime", "PrimaryStatus"}

	flashFields := []string{"FQDD", "Name", "Size", "WriteProtected", "LastUpdateTime", "PrimaryStatus"}

	licFields := []string{"EntitlementID", "LicenseDescription", "LicenseType",
		"LicenseStartDate", "LicenseEndDate", "AllowedDeviceCount", "PrimaryStatus"}

	sessFields := []string{"SessionID", "UserName", "IPAddress", "SessionType", "StartTime"}

	intruFields := []string{"FQDD", "IntrusionType", "IntrusionStatus"}

	lcLogFields := []string{"RecordID", "CreationTimeStamp", "Severity", "Category",
		"AgentID", "MessageID", "Message"}

	fetchers := []fetcher{
		// Hardware
		{key: "system", title: "System", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceSystemView, systemFields)},
		{key: "cpus", title: "CPUs", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceCPUView, cpuFields)},
		{key: "memory", title: "Memory DIMMs", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceMemoryView, memFields)},
		{key: "nics", title: "NICs", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceNICView, nicFields)},
		{key: "fans", title: "Fans", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceFanView, fanFields)},
		{key: "psus", title: "Power Supplies", group: "Hardware", fn: makeEnumFetcher(c, client.ResourcePSView, psuFields)},
		{key: "batteries", title: "Batteries", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceBatteryView, battFields)},
		{key: "sensors", title: "Numeric Sensors", group: "Hardware", collapsed: true, fn: makeEnumFetcher(c, client.ResourceNumericSensor, sensorFields)},
		{key: "front_panel", title: "Front Panel", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceFrontPanelView, fpFields)},
		{key: "flash_media", title: "Flash Media", group: "Hardware", fn: makeEnumFetcher(c, client.ResourceRemovableFlashMedia, flashFields)},
		// Storage
		{key: "controllers", title: "RAID Controllers", group: "Storage", fn: makeEnumFetcher(c, client.ResourceControllerView, ctrlFields)},
		{key: "physical_disks", title: "Physical Disks", group: "Storage", fn: makeEnumFetcher(c, client.ResourcePhysDiskView, pdFields)},
		{key: "virtual_disks", title: "Virtual Disks", group: "Storage", fn: makeEnumFetcher(c, client.ResourceVirtDiskView, vdFields)},
		{key: "enclosures", title: "Enclosures", group: "Storage", fn: makeEnumFetcher(c, client.ResourceEnclosureView, encFields)},
		// Firmware
		{key: "firmware", title: "Firmware Inventory", group: "Firmware", collapsed: true, fn: makeEnumFetcher(c, client.ResourceSoftwareIdentity, fwFields)},
		// Management
		{key: "licenses", title: "Licenses", group: "Management", fn: makeEnumFetcher(c, client.ResourceLicenseManageable, licFields)},
		{key: "sessions", title: "Active Sessions", group: "Management", fn: makeEnumFetcher(c, client.ResourceSessionView, sessFields)},
		{key: "intrusion", title: "Intrusion Detection", group: "Management", fn: makeEnumFetcher(c, client.ResourceIntrusionView, intruFields)},
		{key: "lc_logs", title: "Lifecycle Logs", group: "Management", collapsed: true, fn: makeEnumFetcher(c, client.ResourceLCLogEntry, lcLogFields)},
	}

	fmt.Printf("Fetching %d iDRAC sections from %s (serial — iDRAC 7 limitation)…\n", len(fetchers), *host)
	start := time.Now()
	sections := runFetchers(fetchers)
	fmt.Printf("Done in %s\n", fmtElapsed(time.Since(start)))

	var err error
	if format == "json" {
		err = writeJSON(sections, *out)
	} else {
		err = writeHTML(sections, *host, *out)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nReport written to: %s\n", *out)
}
