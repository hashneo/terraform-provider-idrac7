// datasources_hardware.go — additional hardware data sources covering the full
// iDRAC 7 navigation tree: Batteries, Fans, Front Panel, Removable Flash Media,
// Virtual Disks, Enclosures, Host OS Network Interfaces, and Lifecycle/SEL Logs.
package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
)

// -----------------------------------------------------------------------
// Helper — build a simple list datasource from a DCIM enumerate class.
// Each helper function returns a datasource.DataSource.
// -----------------------------------------------------------------------

// simpleListDS is a generic single-list data source backed by one DCIM class.
type simpleListDS struct {
	client       *client.Client
	typeName     string
	resourceURI  string
	description  string
	fields       []simpleField
}

type simpleField struct {
	name    string // Terraform attribute name
	xmlName string // DCIM XML element name
	desc    string
}

func (d *simpleListDS) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.typeName
}

func (d *simpleListDS) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *simpleListDS) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true},
	}
	itemAttrs := map[string]schema.Attribute{}
	for _, f := range d.fields {
		itemAttrs[f.name] = schema.StringAttribute{Computed: true, MarkdownDescription: f.desc}
	}
	attrs["items"] = schema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: d.description,
		NestedObject:        schema.NestedAttributeObject{Attributes: itemAttrs},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: d.description,
		Attributes:          attrs,
	}
}

func (d *simpleListDS) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Build attr type map
	attrTypes := map[string]attr.Type{}
	for _, f := range d.fields {
		attrTypes[f.name] = types.StringType
	}

	// State model
	type model struct {
		ID    types.String `tfsdk:"id"`
		Items types.List   `tfsdk:"items"`
	}
	var state model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(d.client.Host)

	items, err := d.client.EnumerateAndPull(d.resourceURI)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not enumerate "+d.typeName, err.Error())
		state.Items, _ = types.ListValue(types.ObjectType{AttrTypes: attrTypes}, []attr.Value{})
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	objs := make([]attr.Value, 0, len(items))
	for _, item := range items {
		vals := map[string]attr.Value{}
		for _, f := range d.fields {
			vals[f.name] = types.StringValue(client.FieldValue(item.Raw, f.xmlName))
		}
		obj, diags := types.ObjectValue(attrTypes, vals)
		resp.Diagnostics.Append(diags...)
		objs = append(objs, obj)
	}
	state.Items, _ = types.ListValue(types.ObjectType{AttrTypes: attrTypes}, objs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -----------------------------------------------------------------------
// Batteries
// -----------------------------------------------------------------------

func NewBatteriesDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "batteries",
		resourceURI: client.ResourceBatteryView,
		description: "Reads battery / CMOS / PERC battery status from iDRAC 7 (`DCIM_Battery`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "Name", "Battery name / label."},
			{"primary_status", "PrimaryStatus", "Health status (OK/Warning/Critical)."},
			{"charge_state", "ChargeState", "Current charge state."},
			{"type", "Type", "Battery type (e.g. PERC H710)."},
			{"predicted_capacity", "PredictedCapacity", "Predicted remaining capacity (%)."},
		},
	}
}

// -----------------------------------------------------------------------
// Fans
// -----------------------------------------------------------------------

func NewFansDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "fans",
		resourceURI: client.ResourceFanView,
		description: "Reads fan sensor data from iDRAC 7 (`DCIM_Fan`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "ElementName", "Fan name / label."},
			{"current_reading_rpm", "CurrentReading", "Current fan speed in RPM."},
			{"primary_status", "PrimaryStatus", "Health status (OK/Warning/Critical)."},
			{"lower_critical_rpm", "LowerThresholdCritical", "Lower critical threshold RPM."},
			{"state", "EnabledState", "Enabled state."},
		},
	}
}

// -----------------------------------------------------------------------
// Front Panel
// -----------------------------------------------------------------------

func NewFrontPanelDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "front_panel",
		resourceURI: client.ResourceFrontPanelView,
		description: "Reads front panel management controller info from iDRAC 7 (`DCIM_FrontPanelMgmtControllerView`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"firmware_version", "FirmwareVersion", "Front panel controller firmware version."},
			{"primary_status", "PrimaryStatus", "Health status."},
			{"last_update_time", "LastUpdateTime", "Last firmware update timestamp."},
		},
	}
}

// -----------------------------------------------------------------------
// Removable Flash Media (SD cards, vFlash, etc.)
// -----------------------------------------------------------------------

func NewRemovableFlashMediaDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "removable_flash_media",
		resourceURI: client.ResourceRemovableFlashMedia,
		description: "Reads removable flash media (SD card, vFlash module) from iDRAC 7 (`DCIM_RemovableFlashMediaView`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "Name", "Device name."},
			{"size_mb", "Size", "Capacity in MB."},
			{"primary_status", "PrimaryStatus", "Health status."},
			{"write_protect", "WriteProtected", "Write protection state."},
			{"last_update_time", "LastUpdateTime", "Timestamp of last update."},
		},
	}
}

// -----------------------------------------------------------------------
// Virtual Disks
// -----------------------------------------------------------------------

func NewVirtualDisksDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "virtual_disks",
		resourceURI: client.ResourceVirtDiskView,
		description: "Reads RAID virtual disk configuration from iDRAC 7 (`DCIM_VirtualDiskView`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "Name", "Virtual disk name."},
			{"raid_type", "RAIDTypes", "RAID level (e.g. RAID-1, RAID-5, RAID-10)."},
			{"size_bytes", "SizeInBytes", "Virtual disk size in bytes."},
			{"primary_status", "PrimaryStatus", "Health status."},
			{"raid_status", "RAIDStatus", "RAID operational status (Online/Degraded/Failed)."},
			{"stripe_size", "StripeSize", "Stripe size in bytes."},
			{"read_cache_policy", "ReadCachePolicy", "Read cache policy."},
			{"write_cache_policy", "WriteCachePolicy", "Write cache policy."},
			{"disk_cache_policy", "DiskCachePolicy", "Disk cache policy."},
			{"media_type", "MediaType", "Media type (HDD/SSD)."},
			{"bus_protocol", "BusProtocol", "Bus protocol (SAS/SATA)."},
		},
	}
}

// -----------------------------------------------------------------------
// Enclosures
// -----------------------------------------------------------------------

func NewEnclosuresDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "enclosures",
		resourceURI: client.ResourceEnclosureView,
		description: "Reads storage enclosure information from iDRAC 7 (`DCIM_EnclosureView`).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "Name", "Enclosure name / product string."},
			{"product_name", "ProductName", "Enclosure product name."},
			{"service_tag", "ServiceTag", "Enclosure service tag."},
			{"primary_status", "PrimaryStatus", "Health status."},
			{"slot_count", "SlotCount", "Number of drive slots."},
			{"firmware_version", "FirmwareVersion", "Enclosure firmware version."},
		},
	}
}

// -----------------------------------------------------------------------
// Host OS Network Interfaces
// -----------------------------------------------------------------------

func NewHostOSNetworkDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "host_os_network",
		resourceURI: client.ResourceHostNICView,
		description: "Reads host OS network interface information from iDRAC 7 (`DCIM_HostNetworkInterfaceView`). Requires iDRAC Service Module (iSM) installed in the OS.",
		fields: []simpleField{
			{"fqdd", "FQDD", "Fully qualified device descriptor."},
			{"name", "Name", "Interface name (e.g. eth0, em1)."},
			{"ip_address", "IPv4Address", "Primary IPv4 address."},
			{"ipv6_address", "IPv6Address", "Primary IPv6 address."},
			{"mac_address", "PermanentMACAddress", "MAC address."},
			{"link_speed", "LinkSpeed", "Negotiated link speed."},
			{"duplex", "FullDuplex", "Duplex mode."},
		},
	}
}

// -----------------------------------------------------------------------
// Lifecycle Controller Logs
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*LogsDataSource)(nil)

// LogsDataSource reads Lifecycle Controller and SEL log entries.
type LogsDataSource struct {
	client *client.Client
}

func NewLogsDataSource() datasource.DataSource {
	return &LogsDataSource{}
}

func (d *LogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_logs"
}

func (d *LogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

type logsModel struct {
	ID      types.String `tfsdk:"id"`
	LCLogs  types.List   `tfsdk:"lifecycle_logs"`
	SELLogs types.List   `tfsdk:"sel_logs"`
}

var logAttrTypes = map[string]attr.Type{
	"record_id":    types.StringType,
	"timestamp":    types.StringType,
	"severity":     types.StringType,
	"message":      types.StringType,
	"message_id":   types.StringType,
	"category":     types.StringType,
	"agent":        types.StringType,
}

func (d *LogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	logEntry := schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"record_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "Log record identifier."},
			"timestamp":  schema.StringAttribute{Computed: true, MarkdownDescription: "Log entry timestamp."},
			"severity":   schema.StringAttribute{Computed: true, MarkdownDescription: "Severity (Informational/Warning/Critical)."},
			"message":    schema.StringAttribute{Computed: true, MarkdownDescription: "Log message text."},
			"message_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Message identifier code."},
			"category":   schema.StringAttribute{Computed: true, MarkdownDescription: "Log category (Storage/System/Configuration etc)."},
			"agent":      schema.StringAttribute{Computed: true, MarkdownDescription: "Reporting agent (iDRAC/BIOS/PERC etc)."},
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads Lifecycle Controller logs and System Event Log (SEL) from iDRAC 7.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"lifecycle_logs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Lifecycle Controller log entries (`DCIM_LifecycleLogEntry`).",
				NestedObject:        logEntry,
			},
			"sel_logs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "System Event Log entries (`DCIM_SELLogEntry`).",
				NestedObject:        logEntry,
			},
		},
	}
}

func (d *LogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state logsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(d.client.Host)

	parseLogEntries := func(resourceURI string) types.List {
		items, err := d.client.EnumerateAndPull(resourceURI)
		if err != nil {
			resp.Diagnostics.AddWarning("Could not read logs from "+resourceURI, err.Error())
			list, _ := types.ListValue(types.ObjectType{AttrTypes: logAttrTypes}, []attr.Value{})
			return list
		}
		objs := make([]attr.Value, 0, len(items))
		for _, item := range items {
			r := item.Raw
			obj, diags := types.ObjectValue(logAttrTypes, map[string]attr.Value{
				"record_id":  types.StringValue(client.FieldValue(r, "RecordID")),
				"timestamp":  types.StringValue(client.FieldValue(r, "CreationTimeStamp")),
				"severity":   types.StringValue(client.FieldValue(r, "Severity")),
				"message":    types.StringValue(client.FieldValue(r, "Message")),
				"message_id": types.StringValue(client.FieldValue(r, "MessageID")),
				"category":   types.StringValue(client.FieldValue(r, "Category")),
				"agent":      types.StringValue(client.FieldValue(r, "AgentID")),
			})
			resp.Diagnostics.Append(diags...)
			objs = append(objs, obj)
		}
		list, _ := types.ListValue(types.ObjectType{AttrTypes: logAttrTypes}, objs)
		return list
	}

	state.LCLogs = parseLogEntries(client.ResourceLCLogEntry)
	state.SELLogs = parseLogEntries(client.ResourceSELLogEntry)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
