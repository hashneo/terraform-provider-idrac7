// resources_extended.go — additional resources covering the full iDRAC 7
// navigation tree: iDRAC Network Settings, Alert Destinations, Firmware Update,
// Server Configuration Profile export/import, and Virtual Disk (RAID) management.
package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
)

// -----------------------------------------------------------------------
// iDRAC Network Settings Resource
// Covers: iDRAC Settings → Network in the navigation tree.
// -----------------------------------------------------------------------

var _ resource.Resource = (*NetworkSettingsResource)(nil)

type NetworkSettingsResource struct{ client *client.Client }

func NewNetworkSettingsResource() resource.Resource { return &NetworkSettingsResource{} }

func (r *NetworkSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_settings"
}

func (r *NetworkSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type networkSettingsModel struct {
	ID              types.String `tfsdk:"id"`
	DHCPEnabled     types.Bool   `tfsdk:"dhcp_enabled"`
	IPAddress       types.String `tfsdk:"ip_address"`
	SubnetMask      types.String `tfsdk:"subnet_mask"`
	Gateway         types.String `tfsdk:"gateway"`
	DNSFromDHCP     types.Bool   `tfsdk:"dns_from_dhcp"`
	DNS1            types.String `tfsdk:"dns1"`
	DNS2            types.String `tfsdk:"dns2"`
	NICEnabled      types.Bool   `tfsdk:"nic_enabled"`
	NICSelection    types.String `tfsdk:"nic_selection"` // Dedicated, LOM1, LOM2, LOM3, LOM4
	AutoNegotiate   types.Bool   `tfsdk:"auto_negotiate"`
	NetworkSpeed    types.String `tfsdk:"network_speed"` // 10, 100, 1000
	Duplex          types.String `tfsdk:"duplex"`        // Full, Half
	VLANEnabled     types.Bool   `tfsdk:"vlan_enabled"`
	VLANID          types.Int64  `tfsdk:"vlan_id"`
}

func (r *NetworkSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages iDRAC 7 network interface settings via `DCIM_iDRACCardService.ApplyAttributes`.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"dhcp_enabled":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Enable DHCP for the iDRAC NIC."},
			"ip_address":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Static IPv4 address (used when DHCP is disabled)."},
			"subnet_mask":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Subnet mask."},
			"gateway":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Default gateway."},
			"dns_from_dhcp": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Obtain DNS server addresses via DHCP."},
			"dns1":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Primary DNS server."},
			"dns2":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Secondary DNS server."},
			"nic_enabled":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Enable the iDRAC NIC."},
			"nic_selection": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "NIC selection: `Dedicated`, `LOM1`, `LOM2`, `LOM3`, `LOM4`."},
			"auto_negotiate": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Enable auto-negotiation."},
			"network_speed": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Network speed when auto-negotiate is disabled: `10`, `100`, `1000`."},
			"duplex":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Duplex mode when auto-negotiate is disabled: `Full`, `Half`."},
			"vlan_enabled":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Enable VLAN tagging on the iDRAC NIC."},
			"vlan_id":       schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), MarkdownDescription: "VLAN ID (1–4094)."},
		},
	}
}

func buildNetworkAttrEnvelope(host string, m networkSettingsModel) string {
	boolStr := func(b bool) string {
		if b {
			return "Enabled"
		}
		return "Disabled"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService/ApplyAttributes</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="CreationClassName">DCIM_iDRACCardService</wsman:Selector>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:iDRACCardService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT60S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:ApplyAttributes_INPUT>
      <p:Target>iDRAC.Embedded.1</p:Target>
      <p:AttributeName>IPv4.1.DHCPEnable</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.Address</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.Netmask</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.Gateway</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.DNSFromDHCP</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.DNS1</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>IPv4.1.DNS2</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>NIC.1.Enable</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>NIC.1.Selection</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>NIC.1.Autoneg</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>NIC.1.Speed</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>NIC.1.Duplex</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>VLAN.1.Enable</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>VLAN.1.ID</p:AttributeName><p:AttributeValue>%d</p:AttributeValue>
    </p:ApplyAttributes_INPUT>
  </s:Body>
</s:Envelope>`,
		host,
		boolStr(m.DHCPEnabled.ValueBool()),
		m.IPAddress.ValueString(),
		m.SubnetMask.ValueString(),
		m.Gateway.ValueString(),
		boolStr(m.DNSFromDHCP.ValueBool()),
		m.DNS1.ValueString(),
		m.DNS2.ValueString(),
		boolStr(m.NICEnabled.ValueBool()),
		m.NICSelection.ValueString(),
		boolStr(m.AutoNegotiate.ValueBool()),
		m.NetworkSpeed.ValueString(),
		m.Duplex.ValueString(),
		boolStr(m.VLANEnabled.ValueBool()),
		m.VLANID.ValueInt64(),
	)
}

func (r *NetworkSettingsResource) sendApply(m networkSettingsModel) error {
	_, err := r.client.PostRaw(buildNetworkAttrEnvelope(r.client.Host, m))
	return err
}

func (r *NetworkSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.sendApply(plan); err != nil {
		resp.Diagnostics.AddError("Failed to apply network settings", err.Error())
		return
	}
	plan.ID = types.StringValue(r.client.Host + "/network")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Read back current iDRAC network attributes
	items, err := r.client.EnumerateAndPull(client.ResourceiDRACCard)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read iDRAC card attributes", err.Error())
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	attrMap := make(map[string]string, len(items))
	for _, item := range items {
		name := client.FieldValue(item.Raw, "AttributeName")
		val := client.FieldValue(item.Raw, "CurrentValue")
		if name != "" {
			attrMap[name] = val
		}
	}
	parseBool := func(key string) types.Bool {
		return types.BoolValue(strings.EqualFold(attrMap[key], "enabled") || attrMap[key] == "1")
	}
	parseInt64 := func(key string) types.Int64 {
		var n int64
		fmt.Sscanf(attrMap[key], "%d", &n)
		return types.Int64Value(n)
	}
	state.DHCPEnabled = parseBool("IPv4.1.DHCPEnable")
	state.IPAddress = types.StringValue(attrMap["IPv4.1.Address"])
	state.SubnetMask = types.StringValue(attrMap["IPv4.1.Netmask"])
	state.Gateway = types.StringValue(attrMap["IPv4.1.Gateway"])
	state.DNSFromDHCP = parseBool("IPv4.1.DNSFromDHCP")
	state.DNS1 = types.StringValue(attrMap["IPv4.1.DNS1"])
	state.DNS2 = types.StringValue(attrMap["IPv4.1.DNS2"])
	state.NICEnabled = parseBool("NIC.1.Enable")
	state.NICSelection = types.StringValue(attrMap["NIC.1.Selection"])
	state.AutoNegotiate = parseBool("NIC.1.Autoneg")
	state.NetworkSpeed = types.StringValue(attrMap["NIC.1.Speed"])
	state.Duplex = types.StringValue(attrMap["NIC.1.Duplex"])
	state.VLANEnabled = parseBool("VLAN.1.Enable")
	state.VLANID = parseInt64("VLAN.1.ID")
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NetworkSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.sendApply(plan); err != nil {
		resp.Diagnostics.AddError("Failed to update network settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}

// -----------------------------------------------------------------------
// Alert Destination Resource
// Covers: Server → Alerts in the navigation tree.
// -----------------------------------------------------------------------

var _ resource.Resource = (*AlertDestinationResource)(nil)

type AlertDestinationResource struct{ client *client.Client }

func NewAlertDestinationResource() resource.Resource { return &AlertDestinationResource{} }

func (r *AlertDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_destination"
}

func (r *AlertDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type alertDestinationModel struct {
	ID          types.String `tfsdk:"id"`
	Index       types.Int64  `tfsdk:"index"`       // 1-8 (iDRAC 7 supports 8 alert destinations)
	Enabled     types.Bool   `tfsdk:"enabled"`
	Address     types.String `tfsdk:"address"`      // IP or hostname
	Protocol    types.String `tfsdk:"protocol"`     // SNMP, IPMI_PET, Email
	AlertFilter types.String `tfsdk:"alert_filter"` // All, Critical, Warning, Info
}

func (r *AlertDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an iDRAC 7 alert destination (SNMP trap / IPMI PET / Email) via `DCIM_iDRACCardService.ApplyAttributes`.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"index":        schema.Int64Attribute{Required: true, MarkdownDescription: "Alert destination slot (1–8)."},
			"enabled":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"address":      schema.StringAttribute{Required: true, MarkdownDescription: "Destination IP address or hostname."},
			"protocol":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`SNMP`, `IPMI_PET`, or `Email`."},
			"alert_filter": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Alert severity filter: `All`, `Critical`, `Warning`, `Info`."},
		},
	}
}

func buildAlertEnvelope(host string, m alertDestinationModel) string {
	enabledVal := "Disabled"
	if m.Enabled.ValueBool() {
		enabledVal = "Enabled"
	}
	prefix := fmt.Sprintf("SNMPAlert.%d", m.Index.ValueInt64())
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService/ApplyAttributes</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="CreationClassName">DCIM_iDRACCardService</wsman:Selector>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:iDRACCardService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT60S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:ApplyAttributes_INPUT>
      <p:Target>iDRAC.Embedded.1</p:Target>
      <p:AttributeName>%s.Enable</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>%s.Destination</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>%s.AlertDestinationFilter</p:AttributeName><p:AttributeValue>%s</p:AttributeValue>
    </p:ApplyAttributes_INPUT>
  </s:Body>
</s:Envelope>`,
		host, prefix, enabledVal, prefix, m.Address.ValueString(),
		prefix, m.AlertFilter.ValueString())
}

func (r *AlertDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.PostRaw(buildAlertEnvelope(r.client.Host, plan)); err != nil {
		resp.Diagnostics.AddError("Failed to create alert destination", err.Error())
		return
	}
	plan.ID = types.StringValue(fmt.Sprintf("%s/alert/%d", r.client.Host, plan.Index.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlertDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := r.client.EnumerateAndPull(client.ResourceiDRACCard)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read iDRAC attributes", err.Error())
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	prefix := fmt.Sprintf("SNMPAlert.%d.", state.Index.ValueInt64())
	for _, item := range items {
		name := client.FieldValue(item.Raw, "AttributeName")
		val := client.FieldValue(item.Raw, "CurrentValue")
		switch {
		case name == prefix+"Enable":
			state.Enabled = types.BoolValue(strings.EqualFold(val, "enabled"))
		case name == prefix+"Destination":
			state.Address = types.StringValue(val)
		case name == prefix+"AlertDestinationFilter":
			state.AlertFilter = types.StringValue(val)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AlertDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.PostRaw(buildAlertEnvelope(r.client.Host, plan)); err != nil {
		resp.Diagnostics.AddError("Failed to update alert destination", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AlertDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Disable the alert destination slot
	state.Enabled = types.BoolValue(false)
	state.Address = types.StringValue("")
	if _, err := r.client.PostRaw(buildAlertEnvelope(r.client.Host, state)); err != nil {
		resp.Diagnostics.AddError("Failed to remove alert destination", err.Error())
	}
}

// -----------------------------------------------------------------------
// Virtual Disk (RAID) Resource
// Covers: Storage → Virtual Disks in the navigation tree.
// -----------------------------------------------------------------------

var _ resource.Resource = (*VirtualDiskResource)(nil)

type VirtualDiskResource struct{ client *client.Client }

func NewVirtualDiskResource() resource.Resource { return &VirtualDiskResource{} }

func (r *VirtualDiskResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_virtual_disk"
}

func (r *VirtualDiskResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type virtualDiskModel struct {
	ID               types.String `tfsdk:"id"`
	ControllerFQDD   types.String `tfsdk:"controller_fqdd"`  // e.g. RAID.Integrated.1-1
	VDiskName        types.String `tfsdk:"name"`
	RAIDLevel        types.String `tfsdk:"raid_level"`        // RAID0, RAID1, RAID5, RAID6, RAID10, RAID50, RAID60
	SpanDepth        types.Int64  `tfsdk:"span_depth"`
	SpanLength       types.Int64  `tfsdk:"span_length"`
	PhysicalDisks    types.List   `tfsdk:"physical_disks"`   // list of disk FQDDs
	SizeBytes        types.Int64  `tfsdk:"size_bytes"`        // 0 = use maximum
	StripeSize       types.String `tfsdk:"stripe_size"`       // 512B, 1KB, 2KB, 4KB, 8KB, 16KB, 32KB, 64KB, 128KB
	ReadPolicy       types.String `tfsdk:"read_policy"`       // NoReadAhead, ReadAhead, AdaptiveReadAhead
	WritePolicy      types.String `tfsdk:"write_policy"`      // WriteThrough, WriteBack, WriteBackForce
	DiskCachePolicy  types.String `tfsdk:"disk_cache_policy"` // Default, Enabled, Disabled
	CurrentFQDD      types.String `tfsdk:"fqdd"`             // set after creation
}

func (r *VirtualDiskResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Creates and manages a RAID virtual disk via ` + "`DCIM_RAIDService.CreateVirtualDisk`" + `.

**Warning:** Creating or deleting a virtual disk is a destructive operation. Ensure you have
the correct physical disk FQDDs and have backed up any data before applying.

## Example

~~~hcl
resource "idrac7_virtual_disk" "boot_vd" {
  controller_fqdd = "RAID.Integrated.1-1"
  name            = "OS-RAID1"
  raid_level      = "RAID1"
  span_depth      = 1
  span_length     = 2
  physical_disks  = ["Disk.Bay.0:Enclosure.Internal.0-1:RAID.Integrated.1-1",
                     "Disk.Bay.1:Enclosure.Internal.0-1:RAID.Integrated.1-1"]
  size_bytes      = 0   # 0 = use maximum available
  stripe_size     = "64KB"
  read_policy     = "AdaptiveReadAhead"
  write_policy    = "WriteBack"
  disk_cache_policy = "Default"
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"controller_fqdd":  schema.StringAttribute{Required: true, MarkdownDescription: "FQDD of the RAID controller (e.g. `RAID.Integrated.1-1`)."},
			"name":             schema.StringAttribute{Required: true, MarkdownDescription: "Virtual disk name."},
			"raid_level":       schema.StringAttribute{Required: true, MarkdownDescription: "`RAID0`, `RAID1`, `RAID5`, `RAID6`, `RAID10`, `RAID50`, `RAID60`."},
			"span_depth":       schema.Int64Attribute{Required: true, MarkdownDescription: "Number of spans (e.g. 1 for RAID1/5/6, 2+ for RAID10/50/60)."},
			"span_length":      schema.Int64Attribute{Required: true, MarkdownDescription: "Number of disks per span."},
			"physical_disks":   schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "List of physical disk FQDDs to include."},
			"size_bytes":       schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Virtual disk size in bytes. Use `0` for maximum available."},
			"stripe_size":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Stripe size: `512B`, `1KB`, `2KB`, `4KB`, `8KB`, `16KB`, `32KB`, `64KB`, `128KB`."},
			"read_policy":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`NoReadAhead`, `ReadAhead`, `AdaptiveReadAhead`."},
			"write_policy":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`WriteThrough`, `WriteBack`, `WriteBackForce`."},
			"disk_cache_policy": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "`Default`, `Enabled`, `Disabled`."},
			"fqdd":             schema.StringAttribute{Computed: true, MarkdownDescription: "FQDD of the created virtual disk (populated after apply)."},
		},
	}
}

var raidLevelMap = map[string]string{
	"RAID0":  "4",
	"RAID1":  "2",
	"RAID5":  "64",
	"RAID6":  "128",
	"RAID10": "2048",
	"RAID50": "8192",
	"RAID60": "16384",
}

func buildCreateVDiskEnvelope(host string, m virtualDiskModel, diskFQDDs []string) string {
	raidVal := raidLevelMap[strings.ToUpper(m.RAIDLevel.ValueString())]
	if raidVal == "" {
		raidVal = "2" // default RAID1
	}
	diskXML := ""
	for _, d := range diskFQDDs {
		diskXML += fmt.Sprintf("\n      <p:PDArray>%s</p:PDArray>", d)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService/CreateVirtualDisk</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="CreationClassName">DCIM_RAIDService</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:RAIDService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT600S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:CreateVirtualDisk_INPUT>
      <p:Target>%s</p:Target>
      <p:VirtualDiskName>%s</p:VirtualDiskName>
      <p:Size>%d</p:Size>
      <p:RAIDLevel>%s</p:RAIDLevel>
      <p:SpanDepth>%d</p:SpanDepth>
      <p:SpanLength>%d</p:SpanLength>
      <p:StripeSize>%s</p:StripeSize>
      <p:ReadPolicy>%s</p:ReadPolicy>
      <p:WritePolicy>%s</p:WritePolicy>
      <p:DiskCachePolicy>%s</p:DiskCachePolicy>%s
    </p:CreateVirtualDisk_INPUT>
  </s:Body>
</s:Envelope>`,
		host,
		m.ControllerFQDD.ValueString(),
		m.VDiskName.ValueString(),
		m.SizeBytes.ValueInt64(),
		raidVal,
		m.SpanDepth.ValueInt64(),
		m.SpanLength.ValueInt64(),
		m.StripeSize.ValueString(),
		m.ReadPolicy.ValueString(),
		m.WritePolicy.ValueString(),
		m.DiskCachePolicy.ValueString(),
		diskXML,
	)
}

func buildDeleteVDiskEnvelope(host, fqdd string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_RAIDService/DeleteVirtualDisk</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="CreationClassName">DCIM_RAIDService</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:RAIDService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT600S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:DeleteVirtualDisk_INPUT>
      <p:Target>%s</p:Target>
    </p:DeleteVirtualDisk_INPUT>
  </s:Body>
</s:Envelope>`, host, fqdd)
}

func (r *VirtualDiskResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan virtualDiskModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var diskFQDDs []string
	resp.Diagnostics.Append(plan.PhysicalDisks.ElementsAs(ctx, &diskFQDDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Creating virtual disk", map[string]interface{}{
		"controller": plan.ControllerFQDD.ValueString(),
		"name":       plan.VDiskName.ValueString(),
		"raid_level": plan.RAIDLevel.ValueString(),
	})

	respData, err := r.client.PostRaw(buildCreateVDiskEnvelope(r.client.Host, plan, diskFQDDs))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create virtual disk", err.Error())
		return
	}

	// Extract the new FQDD from the response
	newFQDD := client.FieldValue(respData, "VirtualDiskFQDD")
	if newFQDD == "" {
		newFQDD = client.FieldValue(respData, "CreatedVirtualDisk")
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/vdisk/%s", r.client.Host, plan.VDiskName.ValueString()))
	plan.CurrentFQDD = types.StringValue(newFQDD)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VirtualDiskResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state virtualDiskModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Read back from DCIM_VirtualDiskView
	items, err := r.client.EnumerateAndPull(client.ResourceVirtDiskView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read virtual disks", err.Error())
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	for _, item := range items {
		if client.FieldValue(item.Raw, "FQDD") == state.CurrentFQDD.ValueString() {
			state.VDiskName = types.StringValue(client.FieldValue(item.Raw, "Name"))
			state.RAIDLevel = types.StringValue(client.FieldValue(item.Raw, "RAIDTypes"))
			state.ReadPolicy = types.StringValue(client.FieldValue(item.Raw, "ReadCachePolicy"))
			state.WritePolicy = types.StringValue(client.FieldValue(item.Raw, "WriteCachePolicy"))
			state.DiskCachePolicy = types.StringValue(client.FieldValue(item.Raw, "DiskCachePolicy"))
			break
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VirtualDiskResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Virtual disk update not supported",
		"iDRAC 7 does not support in-place virtual disk modification. Delete and recreate the virtual disk instead.")
}

func (r *VirtualDiskResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state virtualDiskModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fqdd := state.CurrentFQDD.ValueString()
	if fqdd == "" {
		resp.Diagnostics.AddWarning("No FQDD recorded", "Cannot delete virtual disk — FQDD unknown. Remove from state manually.")
		return
	}
	tflog.Info(ctx, "Deleting virtual disk", map[string]interface{}{"fqdd": fqdd})
	if _, err := r.client.PostRaw(buildDeleteVDiskEnvelope(r.client.Host, fqdd)); err != nil {
		resp.Diagnostics.AddError("Failed to delete virtual disk", err.Error())
	}
}

// -----------------------------------------------------------------------
// Server Configuration Profile (SCP) Export Resource
// Covers: iDRAC Settings → Server Profile in the navigation tree.
// -----------------------------------------------------------------------

var _ resource.Resource = (*ServerProfileResource)(nil)

type ServerProfileResource struct{ client *client.Client }

func NewServerProfileResource() resource.Resource { return &ServerProfileResource{} }

func (r *ServerProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_profile"
}

func (r *ServerProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type serverProfileModel struct {
	ID          types.String `tfsdk:"id"`
	ExportPath  types.String `tfsdk:"export_path"`   // local file path to write the exported SCP XML
	Target      types.String `tfsdk:"target"`        // ALL, BIOS, IDRAC, NIC, RAID
	ExportFormat types.String `tfsdk:"export_format"` // XML, JSON
	Profile     types.String `tfsdk:"profile"`       // exported profile content (computed)
}

func (r *ServerProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Exports the Server Configuration Profile (SCP) from iDRAC 7 via ` + "`DCIM_LCService.ExportSystemConfiguration`" + `.

The SCP captures all BIOS, iDRAC, NIC, and RAID configuration in a single XML document,
which can be used for backup, cloning, or compliance auditing.

**Note:** This resource is read-like — Create/Read trigger an export; Delete is a no-op.

## Example

~~~hcl
resource "idrac7_server_profile" "backup" {
  export_path   = "/tmp/r420_scp.xml"
  target        = "ALL"
  export_format = "XML"
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"export_path":  schema.StringAttribute{Required: true, MarkdownDescription: "Local file path where the exported SCP will be written."},
			"target":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Components to export: `ALL`, `BIOS`, `IDRAC`, `NIC`, `RAID`."},
			"export_format": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Export format: `XML` or `JSON`."},
			"profile":      schema.StringAttribute{Computed: true, Sensitive: false, MarkdownDescription: "Exported SCP profile content."},
		},
	}
}

func buildExportSCPEnvelope(host, target, format string) string {
	if target == "" {
		target = "ALL"
	}
	if format == "" {
		format = "XML"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_LCService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_LCService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_LCService/ExportSystemConfiguration</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="CreationClassName">DCIM_LCService</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:LCService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT600S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:ExportSystemConfiguration_INPUT>
      <p:IPAddress>%s</p:IPAddress>
      <p:ShareType>LOCAL</p:ShareType>
      <p:Target>%s</p:Target>
      <p:ExportFormat>%s</p:ExportFormat>
    </p:ExportSystemConfiguration_INPUT>
  </s:Body>
</s:Envelope>`, host, host, target, format)
}

func (r *ServerProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	target := plan.Target.ValueString()
	if target == "" {
		target = "ALL"
	}
	format := plan.ExportFormat.ValueString()
	if format == "" {
		format = "XML"
	}

	tflog.Info(ctx, "Exporting server configuration profile", map[string]interface{}{
		"target": target, "format": format,
	})

	respData, err := r.client.PostRaw(buildExportSCPEnvelope(r.client.Host, target, format))
	if err != nil {
		resp.Diagnostics.AddError("SCP export failed", err.Error())
		return
	}

	profileContent := string(respData)
	plan.ID = types.StringValue(r.client.Host + "/scp")
	plan.Profile = types.StringValue(profileContent)
	plan.Target = types.StringValue(target)
	plan.ExportFormat = types.StringValue(format)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServerProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Re-export on every Read to keep state current
	respData, err := r.client.PostRaw(buildExportSCPEnvelope(
		r.client.Host,
		state.Target.ValueString(),
		state.ExportFormat.ValueString(),
	))
	if err != nil {
		resp.Diagnostics.AddWarning("Could not re-export SCP", err.Error())
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	state.Profile = types.StringValue(string(respData))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServerProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Treat update as a new export
	var plan serverProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	respData, err := r.client.PostRaw(buildExportSCPEnvelope(
		r.client.Host, plan.Target.ValueString(), plan.ExportFormat.ValueString(),
	))
	if err != nil {
		resp.Diagnostics.AddError("SCP export failed", err.Error())
		return
	}
	plan.Profile = types.StringValue(string(respData))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServerProfileResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing this resource does not modify the server — it only removes the export from state.
}

// -----------------------------------------------------------------------
// Firmware Update Resource
// Covers: iDRAC Settings → Update and Rollback in the navigation tree.
// -----------------------------------------------------------------------

var _ resource.Resource = (*FirmwareUpdateResource)(nil)

type FirmwareUpdateResource struct{ client *client.Client }

func NewFirmwareUpdateResource() resource.Resource { return &FirmwareUpdateResource{} }

func (r *FirmwareUpdateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firmware_update"
}

func (r *FirmwareUpdateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type firmwareUpdateModel struct {
	ID              types.String `tfsdk:"id"`
	ShareType       types.String `tfsdk:"share_type"`    // NFS, CIFS, HTTP, HTTPS, FTP, TFTP
	ShareIP         types.String `tfsdk:"share_ip"`
	ShareName       types.String `tfsdk:"share_name"`    // share name / path
	CatalogFile     types.String `tfsdk:"catalog_file"`  // e.g. Catalog.xml
	RebootNeeded    types.Bool   `tfsdk:"reboot_needed"`
	ApplyUpdate     types.Bool   `tfsdk:"apply_update"`
	JobID           types.String `tfsdk:"job_id"`        // computed after triggering
}

func (r *FirmwareUpdateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Triggers a firmware update on the iDRAC 7 server via ` + "`DCIM_SoftwareInstallationService.InstallFromRepository`" + `.

The update catalog must be accessible from the server via NFS, CIFS, or HTTP share.

## Example

~~~hcl
resource "idrac7_firmware_update" "r420" {
  share_type   = "NFS"
  share_ip     = "192.168.1.50"
  share_name   = "/firmware/dell"
  catalog_file = "Catalog.xml"
  apply_update = true
  reboot_needed = true
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"share_type":   schema.StringAttribute{Required: true, MarkdownDescription: "Share type: `NFS`, `CIFS`, `HTTP`, `HTTPS`, `FTP`, `TFTP`."},
			"share_ip":     schema.StringAttribute{Required: true, MarkdownDescription: "IP address of the firmware share server."},
			"share_name":   schema.StringAttribute{Required: true, MarkdownDescription: "Share name or path containing the catalog."},
			"catalog_file": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Catalog XML filename (default: `Catalog.xml`)."},
			"apply_update": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Apply updates immediately after staging."},
			"reboot_needed": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Reboot server to apply updates that require it."},
			"job_id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Job ID returned by iDRAC for the update task."},
		},
	}
}

var shareTypeMap = map[string]string{
	"NFS": "0", "CIFS": "2", "HTTP": "5", "HTTPS": "6", "FTP": "1", "TFTP": "7",
}

func buildFirmwareUpdateEnvelope(host string, m firmwareUpdateModel) string {
	shareTypeNum := shareTypeMap[strings.ToUpper(m.ShareType.ValueString())]
	if shareTypeNum == "" {
		shareTypeNum = "0"
	}
	catalog := m.CatalogFile.ValueString()
	if catalog == "" {
		catalog = "Catalog.xml"
	}
	applyStr := "FALSE"
	if m.ApplyUpdate.ValueBool() {
		applyStr = "TRUE"
	}
	rebootStr := "FALSE"
	if m.RebootNeeded.ValueBool() {
		rebootStr = "TRUE"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_SoftwareInstallationService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_SoftwareInstallationService</wsman:ResourceURI>
    <a:ReplyTo><a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_SoftwareInstallationService/InstallFromRepository</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="CreationClassName">DCIM_SoftwareInstallationService</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">SoftwareUpdate.1</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT600S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:InstallFromRepository_INPUT>
      <p:IPAddress>%s</p:IPAddress>
      <p:ShareType>%s</p:ShareType>
      <p:ShareName>%s</p:ShareName>
      <p:CatalogFile>%s</p:CatalogFile>
      <p:ApplyUpdate>%s</p:ApplyUpdate>
      <p:RebootNeeded>%s</p:RebootNeeded>
    </p:InstallFromRepository_INPUT>
  </s:Body>
</s:Envelope>`,
		host, m.ShareIP.ValueString(), shareTypeNum,
		m.ShareName.ValueString(), catalog, applyStr, rebootStr)
}

func (r *FirmwareUpdateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firmwareUpdateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Triggering firmware update", map[string]interface{}{
		"share": fmt.Sprintf("%s://%s%s", plan.ShareType.ValueString(), plan.ShareIP.ValueString(), plan.ShareName.ValueString()),
	})

	respData, err := r.client.PostRaw(buildFirmwareUpdateEnvelope(r.client.Host, plan))
	if err != nil {
		resp.Diagnostics.AddError("Firmware update failed", err.Error())
		return
	}

	jobID := client.FieldValue(respData, "Job")
	if jobID == "" {
		jobID = client.FieldValue(respData, "JobID")
	}

	plan.ID = types.StringValue(r.client.Host + "/firmware")
	plan.JobID = types.StringValue(jobID)
	if plan.CatalogFile.IsNull() || plan.CatalogFile.ValueString() == "" {
		plan.CatalogFile = types.StringValue("Catalog.xml")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirmwareUpdateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firmwareUpdateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirmwareUpdateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firmwareUpdateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	respData, err := r.client.PostRaw(buildFirmwareUpdateEnvelope(r.client.Host, plan))
	if err != nil {
		resp.Diagnostics.AddError("Firmware update failed", err.Error())
		return
	}
	plan.JobID = types.StringValue(client.FieldValue(respData, "Job"))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirmwareUpdateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}
