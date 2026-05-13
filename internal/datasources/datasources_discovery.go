// datasources_discovery.go — discovery-focused data sources that require zero
// prior knowledge of the server. These allow a complete blind inventory of the
// server by simply pointing the provider at an iDRAC 7 IP and running plan/apply.
//
// Data sources in this file:
//   - idrac7_firmware_inventory  — all installed firmware versions per component
//   - idrac7_bios_all            — full current BIOS attribute key/value map
//   - idrac7_intrusion           — chassis intrusion detection status
//   - idrac7_licenses            — installed iDRAC feature licences
//   - idrac7_sessions            — active iDRAC sessions
//   - idrac7_discovery           — single data source that runs ALL of the above
//                                  plus hardware inventory and returns a complete
//                                  structured snapshot of the entire server
package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
)

// -----------------------------------------------------------------------
// Firmware Inventory
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*FirmwareInventoryDataSource)(nil)

type FirmwareInventoryDataSource struct{ client *client.Client }

func NewFirmwareInventoryDataSource() datasource.DataSource {
	return &FirmwareInventoryDataSource{}
}

func (d *FirmwareInventoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firmware_inventory"
}

func (d *FirmwareInventoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

var firmwareAttrTypes = map[string]attr.Type{
	"fqdd":            types.StringType,
	"component_id":    types.StringType,
	"element_name":    types.StringType,
	"version":         types.StringType,
	"status":          types.StringType,
	"update_required": types.StringType,
	"instance_id":     types.StringType,
}

type firmwareInventoryModel struct {
	ID        types.String `tfsdk:"id"`
	Firmware  types.List   `tfsdk:"firmware"`
}

func (d *FirmwareInventoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all installed firmware versions from iDRAC 7 via `DCIM_SoftwareIdentity`. Returns versions for every installed component: iDRAC, BIOS, PERC, NICs, drives, etc.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"firmware": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all installed firmware components.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":            schema.StringAttribute{Computed: true, MarkdownDescription: "Component FQDD."},
						"component_id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Dell component identifier."},
						"element_name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable component name."},
						"version":         schema.StringAttribute{Computed: true, MarkdownDescription: "Installed firmware version string."},
						"status":          schema.StringAttribute{Computed: true, MarkdownDescription: "Installation status."},
						"update_required": schema.StringAttribute{Computed: true, MarkdownDescription: "Whether an update is available/required."},
						"instance_id":     schema.StringAttribute{Computed: true, MarkdownDescription: "WS-MAN instance identifier."},
					},
				},
			},
		},
	}
}

func (d *FirmwareInventoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state firmwareInventoryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(d.client.Host)

	items, err := d.client.EnumerateAndPull(client.ResourceSoftwareIdentity)
	if err != nil {
		resp.Diagnostics.AddError("Could not read firmware inventory", err.Error())
		return
	}

	objs := make([]attr.Value, 0, len(items))
	for _, item := range items {
		r := item.Raw
		obj, diags := types.ObjectValue(firmwareAttrTypes, map[string]attr.Value{
			"fqdd":            types.StringValue(client.FieldValue(r, "FQDD")),
			"component_id":    types.StringValue(client.FieldValue(r, "ComponentID")),
			"element_name":    types.StringValue(client.FieldValue(r, "ElementName")),
			"version":         types.StringValue(client.FieldValue(r, "VersionString")),
			"status":          types.StringValue(client.FieldValue(r, "Status")),
			"update_required": types.StringValue(client.FieldValue(r, "UpdateRequired")),
			"instance_id":     types.StringValue(client.FieldValue(r, "InstanceID")),
		})
		resp.Diagnostics.Append(diags...)
		objs = append(objs, obj)
	}

	state.Firmware, _ = types.ListValue(types.ObjectType{AttrTypes: firmwareAttrTypes}, objs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -----------------------------------------------------------------------
// BIOS All Attributes
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*BIOSAllDataSource)(nil)

type BIOSAllDataSource struct{ client *client.Client }

func NewBIOSAllDataSource() datasource.DataSource { return &BIOSAllDataSource{} }

func (d *BIOSAllDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bios_all"
}

func (d *BIOSAllDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

type biosAllModel struct {
	ID         types.String `tfsdk:"id"`
	Attributes types.Map    `tfsdk:"attributes"` // map[string]string — all current BIOS key/values
	Metadata   types.List   `tfsdk:"metadata"`   // list of attribute metadata (type, possible values, etc.)
}

var biosMetaAttrTypes = map[string]attr.Type{
	"name":           types.StringType,
	"current_value":  types.StringType,
	"pending_value":  types.StringType,
	"attribute_type": types.StringType,
	"read_only":      types.StringType,
	"possible_values": types.StringType,
	"min_value":      types.StringType,
	"max_value":      types.StringType,
}

func (d *BIOSAllDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads **all** current BIOS attributes from iDRAC 7. Returns both a simple `attributes` map (name→value) and full `metadata` (type, allowed values, read-only flag) for every BIOS setting.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"attributes": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of all BIOS attribute names to their current values.",
			},
			"metadata": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Full metadata for each BIOS attribute.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":            schema.StringAttribute{Computed: true, MarkdownDescription: "Attribute name."},
						"current_value":   schema.StringAttribute{Computed: true, MarkdownDescription: "Current committed value."},
						"pending_value":   schema.StringAttribute{Computed: true, MarkdownDescription: "Pending value (will apply on next boot)."},
						"attribute_type":  schema.StringAttribute{Computed: true, MarkdownDescription: "Attribute type: Enumeration, String, Integer."},
						"read_only":       schema.StringAttribute{Computed: true, MarkdownDescription: "Whether the attribute is read-only."},
						"possible_values": schema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated list of allowed values (for Enumeration type)."},
						"min_value":       schema.StringAttribute{Computed: true, MarkdownDescription: "Minimum value (for Integer type)."},
						"max_value":       schema.StringAttribute{Computed: true, MarkdownDescription: "Maximum value (for Integer type)."},
					},
				},
			},
		},
	}
}

func (d *BIOSAllDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state biosAllModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(d.client.Host)

	attrMap := make(map[string]string)
	metaObjs := make([]attr.Value, 0, 256)

	for _, resClass := range []string{
		client.ResourceBIOSEnum,
		client.ResourceBIOSString,
		client.ResourceBIOSInteger,
	} {
		attrType := "Enumeration"
		if resClass == client.ResourceBIOSString {
			attrType = "String"
		} else if resClass == client.ResourceBIOSInteger {
			attrType = "Integer"
		}

		items, err := d.client.EnumerateAndPull(resClass)
		if err != nil {
			resp.Diagnostics.AddWarning("Could not enumerate BIOS class "+resClass, err.Error())
			continue
		}

		for _, item := range items {
			r := item.Raw
			name := client.FieldValue(r, "AttributeName")
			current := client.FieldValue(r, "CurrentValue")
			if name == "" {
				continue
			}
			attrMap[name] = current

			// Collect possible values for enumerations (repeated PossibleValuesDescription elements)
			possibleVals := ""
			if attrType == "Enumeration" {
				vals := client.AllFieldValues(r, "PossibleValuesDescription")
				for i, v := range vals {
					if i > 0 {
						possibleVals += ","
					}
					possibleVals += v
				}
			}

			obj, diags := types.ObjectValue(biosMetaAttrTypes, map[string]attr.Value{
				"name":            types.StringValue(name),
				"current_value":   types.StringValue(current),
				"pending_value":   types.StringValue(client.FieldValue(r, "PendingValue")),
				"attribute_type":  types.StringValue(attrType),
				"read_only":       types.StringValue(client.FieldValue(r, "IsReadOnly")),
				"possible_values": types.StringValue(possibleVals),
				"min_value":       types.StringValue(client.FieldValue(r, "LowerBound")),
				"max_value":       types.StringValue(client.FieldValue(r, "UpperBound")),
			})
			resp.Diagnostics.Append(diags...)
			metaObjs = append(metaObjs, obj)
		}
	}

	attrMapVal, diags := types.MapValueFrom(ctx, types.StringType, attrMap)
	resp.Diagnostics.Append(diags...)
	state.Attributes = attrMapVal
	state.Metadata, _ = types.ListValue(types.ObjectType{AttrTypes: biosMetaAttrTypes}, metaObjs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -----------------------------------------------------------------------
// Intrusion Detection
// -----------------------------------------------------------------------

func NewIntrusionDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "intrusion",
		resourceURI: client.ResourceIntrusionView,
		description: "Reads chassis intrusion detection status from iDRAC 7 (`DCIM_PhysicalPackage` intrusion sensor).",
		fields: []simpleField{
			{"fqdd", "FQDD", "Component FQDD."},
			{"intrusion_type", "IntrusionType", "Intrusion type (ChassisBreech etc)."},
			{"intrusion_status", "IntrusionStatus", "Current intrusion status (Normal/Detected/NotApplicable)."},
		},
	}
}

// -----------------------------------------------------------------------
// Licenses
// -----------------------------------------------------------------------

func NewLicensesDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "licenses",
		resourceURI: client.ResourceLicenseManageable,
		description: "Reads installed iDRAC feature licenses from iDRAC 7 (`DCIM_LicenseManageable`).",
		fields: []simpleField{
			{"instance_id", "InstanceID", "License instance identifier."},
			{"entitlement_id", "EntitlementID", "License entitlement ID."},
			{"license_description", "LicenseDescription", "Human-readable license description."},
			{"license_type", "LicenseType", "License type (Perpetual/Leased/Evaluation/Site)."},
			{"primary_status", "PrimaryStatus", "License health status."},
			{"license_start_date", "LicenseStartDate", "License start date."},
			{"license_end_date", "LicenseEndDate", "License expiry date (if applicable)."},
			{"allowed_device_count", "AllowedDeviceCount", "Number of devices this licence covers."},
		},
	}
}

// -----------------------------------------------------------------------
// Sessions
// -----------------------------------------------------------------------

func NewSessionsDataSource() datasource.DataSource {
	return &simpleListDS{
		typeName:    "sessions",
		resourceURI: client.ResourceSessionView,
		description: "Reads active iDRAC 7 sessions (`DCIM_SessionView`).",
		fields: []simpleField{
			{"session_id", "SessionID", "Session identifier."},
			{"username", "UserName", "Username of the session."},
			{"ip_address", "IPAddress", "Source IP address."},
			{"session_type", "SessionType", "Session type (GUI/SSH/Telnet/RACADM/IPMI/WSMAN etc)."},
			{"start_time", "StartTime", "Session start timestamp."},
		},
	}
}

// -----------------------------------------------------------------------
// Discovery — full server snapshot (zero prior knowledge required)
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*DiscoveryDataSource)(nil)

// DiscoveryDataSource performs a complete discovery of the server — every
// hardware component, firmware version, BIOS setting, sensor, storage topology,
// network, and iDRAC config — in a single data source read.
type DiscoveryDataSource struct{ client *client.Client }

func NewDiscoveryDataSource() datasource.DataSource { return &DiscoveryDataSource{} }

func (d *DiscoveryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discovery"
}

func (d *DiscoveryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// discoveryModel is the complete server snapshot.
type discoveryModel struct {
	ID   types.String `tfsdk:"id"`

	// Identity
	ServiceTag      types.String `tfsdk:"service_tag"`
	Model           types.String `tfsdk:"model"`
	Manufacturer    types.String `tfsdk:"manufacturer"`
	BIOSVersion     types.String `tfsdk:"bios_version"`
	IDRACFirmware   types.String `tfsdk:"idrac_firmware_version"`
	PowerState      types.String `tfsdk:"power_state"`
	Hostname        types.String `tfsdk:"hostname"`
	OSName          types.String `tfsdk:"os_name"`
	MemoryTotalMB   types.Int64  `tfsdk:"memory_total_mb"`
	CPUCount        types.Int64  `tfsdk:"cpu_count"`

	// Hardware components — FQDDs useful for resource authoring
	ControllerFQDDs   types.List `tfsdk:"controller_fqdds"`    // []string — ready to paste into idrac7_virtual_disk
	PhysicalDiskFQDDs types.List `tfsdk:"physical_disk_fqdds"` // []string
	NICFQDDs          types.List `tfsdk:"nic_fqdds"`           // []string
	VirtualDiskFQDDs  types.List `tfsdk:"virtual_disk_fqdds"`  // []string

	// Firmware — map[fqdd]version
	FirmwareVersions  types.Map  `tfsdk:"firmware_versions"`

	// BIOS — map[attr_name]current_value
	BIOSAttributes    types.Map  `tfsdk:"bios_attributes"`

	// Sensor summary
	FanCount          types.Int64  `tfsdk:"fan_count"`
	PSUCount          types.Int64  `tfsdk:"psu_count"`
	BatteryCount      types.Int64  `tfsdk:"battery_count"`
	CriticalSensors   types.List   `tfsdk:"critical_sensors"` // []string names of sensors in critical state

	// Alert destinations configured ([]string of destination IPs)
	AlertDestinations types.List `tfsdk:"alert_destinations"`

	// Active sessions count
	ActiveSessions    types.Int64 `tfsdk:"active_sessions"`

	// Licences ([]string of descriptions)
	Licenses          types.List `tfsdk:"licenses"`

	// Intrusion status
	IntrusionStatus   types.String `tfsdk:"intrusion_status"`
}

func (d *DiscoveryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `**Full server discovery** — requires zero prior knowledge of the server.

Point the provider at an iDRAC 7 IP and this data source returns a complete structured
snapshot of the entire server: identity, all hardware FQDDs, firmware versions,
current BIOS settings, sensor state, storage topology, network, licenses, and sessions.

The ` + "`controller_fqdds`" + ` and ` + "`physical_disk_fqdds`" + ` outputs can be used directly
as inputs to ` + "`idrac7_virtual_disk`" + ` resources, enabling fully dynamic RAID configuration
without any hardcoded values.

## Example

~~~hcl
data "idrac7_discovery" "server" {}

output "full_snapshot" {
  value = data.idrac7_discovery.server
}

# Use discovered FQDDs directly in a virtual disk resource
resource "idrac7_virtual_disk" "os_raid" {
  controller_fqdd = data.idrac7_discovery.server.controller_fqdds[0]
  physical_disks  = slice(data.idrac7_discovery.server.physical_disk_fqdds, 0, 2)
  name            = "OS-RAID1"
  raid_level      = "RAID1"
  span_depth      = 1
  span_length     = 2
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id":                    schema.StringAttribute{Computed: true},
			"service_tag":           schema.StringAttribute{Computed: true},
			"model":                 schema.StringAttribute{Computed: true},
			"manufacturer":          schema.StringAttribute{Computed: true},
			"bios_version":          schema.StringAttribute{Computed: true},
			"idrac_firmware_version": schema.StringAttribute{Computed: true},
			"power_state":           schema.StringAttribute{Computed: true},
			"hostname":              schema.StringAttribute{Computed: true},
			"os_name":               schema.StringAttribute{Computed: true},
			"memory_total_mb":       schema.Int64Attribute{Computed: true},
			"cpu_count":             schema.Int64Attribute{Computed: true},
			"controller_fqdds":      schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "List of storage controller FQDDs — use directly in `idrac7_virtual_disk.controller_fqdd`."},
			"physical_disk_fqdds":   schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "List of physical disk FQDDs — use directly in `idrac7_virtual_disk.physical_disks`."},
			"nic_fqdds":             schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "List of NIC FQDDs."},
			"virtual_disk_fqdds":    schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "List of existing virtual disk FQDDs."},
			"firmware_versions":     schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Map of component FQDD → installed firmware version."},
			"bios_attributes":       schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Map of all current BIOS attribute names → values."},
			"fan_count":             schema.Int64Attribute{Computed: true},
			"psu_count":             schema.Int64Attribute{Computed: true},
			"battery_count":         schema.Int64Attribute{Computed: true},
			"critical_sensors":      schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Names of sensors currently in a critical or warning state."},
			"alert_destinations":    schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Configured alert destination IP addresses."},
			"active_sessions":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of currently active iDRAC sessions."},
			"licenses":              schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Installed iDRAC license descriptions."},
			"intrusion_status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Chassis intrusion status (Normal/Detected/NotApplicable)."},
		},
	}
}

// strList converts a slice of strings to a types.List.
func strList(ss []string) types.List {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	l, _ := types.ListValue(types.StringType, vals)
	return l
}

func (d *DiscoveryDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	state := discoveryModel{}
	state.ID = types.StringValue(d.client.Host)

	tflog.Info(ctx, "Running full iDRAC 7 discovery", map[string]interface{}{"host": d.client.Host})

	// ---- System info ----
	if sysItems, err := d.client.EnumerateAndPull(client.ResourceSystemView); err == nil && len(sysItems) > 0 {
		r := sysItems[0].Raw
		svcTag := client.FieldValue(r, "ServiceTag")
		state.ServiceTag = types.StringValue(svcTag)
		state.Model = types.StringValue(client.FieldValue(r, "Model"))
		state.Manufacturer = types.StringValue(client.FieldValue(r, "Manufacturer"))
		state.BIOSVersion = types.StringValue(client.FieldValue(r, "BIOSVersionString"))
		state.IDRACFirmware = types.StringValue(client.FieldValue(r, "LifecycleControllerVersion"))
		state.PowerState = types.StringValue(client.FieldValue(r, "PowerState"))
		state.Hostname = types.StringValue(client.FieldValue(r, "HostName"))
		state.OSName = types.StringValue(client.FieldValue(r, "OSName"))
		var memMB, cpuCt int64
		fmt.Sscanf(client.FieldValue(r, "SysMemTotalSize"), "%d", &memMB)
		fmt.Sscanf(client.FieldValue(r, "CPUSocketsPopulated"), "%d", &cpuCt)
		state.MemoryTotalMB = types.Int64Value(memMB)
		state.CPUCount = types.Int64Value(cpuCt)
	} else {
		resp.Diagnostics.AddWarning("Could not read system view", fmt.Sprintf("%v", err))
		state.ServiceTag = types.StringValue("")
		state.Model = types.StringValue("")
		state.Manufacturer = types.StringValue("")
		state.BIOSVersion = types.StringValue("")
		state.IDRACFirmware = types.StringValue("")
		state.PowerState = types.StringValue("")
		state.Hostname = types.StringValue("")
		state.OSName = types.StringValue("")
		state.MemoryTotalMB = types.Int64Value(0)
		state.CPUCount = types.Int64Value(0)
	}

	// ---- Controller FQDDs ----
	ctrlFQDDs := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceControllerView); err == nil {
		for _, item := range items {
			if fqdd := client.FieldValue(item.Raw, "FQDD"); fqdd != "" {
				ctrlFQDDs = append(ctrlFQDDs, fqdd)
			}
		}
	}
	state.ControllerFQDDs = strList(ctrlFQDDs)

	// ---- Physical disk FQDDs ----
	pdFQDDs := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourcePhysDiskView); err == nil {
		for _, item := range items {
			if fqdd := client.FieldValue(item.Raw, "FQDD"); fqdd != "" {
				pdFQDDs = append(pdFQDDs, fqdd)
			}
		}
	}
	state.PhysicalDiskFQDDs = strList(pdFQDDs)

	// ---- NIC FQDDs ----
	nicFQDDs := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceNICView); err == nil {
		for _, item := range items {
			if fqdd := client.FieldValue(item.Raw, "FQDD"); fqdd != "" {
				nicFQDDs = append(nicFQDDs, fqdd)
			}
		}
	}
	state.NICFQDDs = strList(nicFQDDs)

	// ---- Virtual disk FQDDs ----
	vdFQDDs := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceVirtDiskView); err == nil {
		for _, item := range items {
			if fqdd := client.FieldValue(item.Raw, "FQDD"); fqdd != "" {
				vdFQDDs = append(vdFQDDs, fqdd)
			}
		}
	}
	state.VirtualDiskFQDDs = strList(vdFQDDs)

	// ---- Firmware versions — map[fqdd]version ----
	fwMap := make(map[string]string)
	if items, err := d.client.EnumerateAndPull(client.ResourceSoftwareIdentity); err == nil {
		for _, item := range items {
			r := item.Raw
			fqdd := client.FieldValue(r, "FQDD")
			ver := client.FieldValue(r, "VersionString")
			if fqdd == "" {
				fqdd = client.FieldValue(r, "ElementName")
			}
			if fqdd != "" {
				fwMap[fqdd] = ver
			}
		}
	}
	fwMapVal, diags := types.MapValueFrom(ctx, types.StringType, fwMap)
	resp.Diagnostics.Append(diags...)
	state.FirmwareVersions = fwMapVal

	// ---- BIOS attributes — map[name]current_value ----
	biosMap := make(map[string]string)
	for _, resClass := range []string{client.ResourceBIOSEnum, client.ResourceBIOSString, client.ResourceBIOSInteger} {
		if items, err := d.client.EnumerateAndPull(resClass); err == nil {
			for _, item := range items {
				name := client.FieldValue(item.Raw, "AttributeName")
				val := client.FieldValue(item.Raw, "CurrentValue")
				if name != "" {
					biosMap[name] = val
				}
			}
		}
	}
	biosMapVal, diags := types.MapValueFrom(ctx, types.StringType, biosMap)
	resp.Diagnostics.Append(diags...)
	state.BIOSAttributes = biosMapVal

	// ---- Sensors — count fans/PSUs, collect critical ----
	var fanCount, psuCount int64
	criticalSensors := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceFanView); err == nil {
		fanCount = int64(len(items))
		for _, item := range items {
			st := client.FieldValue(item.Raw, "PrimaryStatus")
			if st == "2" || st == "3" { // Warning or Critical
				criticalSensors = append(criticalSensors, client.FieldValue(item.Raw, "ElementName"))
			}
		}
	}
	if items, err := d.client.EnumerateAndPull(client.ResourcePSView); err == nil {
		psuCount = int64(len(items))
		for _, item := range items {
			st := client.FieldValue(item.Raw, "PrimaryStatus")
			if st == "2" || st == "3" {
				criticalSensors = append(criticalSensors, client.FieldValue(item.Raw, "ProductName"))
			}
		}
	}
	if items, err := d.client.EnumerateAndPull(client.ResourceNumericSensor); err == nil {
		for _, item := range items {
			st := client.FieldValue(item.Raw, "HealthState")
			if st == "20" || st == "25" { // Major/Critical in CIM HealthState
				criticalSensors = append(criticalSensors, client.FieldValue(item.Raw, "ElementName"))
			}
		}
	}
	state.FanCount = types.Int64Value(fanCount)
	state.PSUCount = types.Int64Value(psuCount)
	state.CriticalSensors = strList(criticalSensors)

	// ---- Batteries ----
	if items, err := d.client.EnumerateAndPull(client.ResourceBatteryView); err == nil {
		state.BatteryCount = types.Int64Value(int64(len(items)))
	} else {
		state.BatteryCount = types.Int64Value(0)
	}

	// ---- Alert destinations ----
	alertDests := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceiDRACCard); err == nil {
		for _, item := range items {
			name := client.FieldValue(item.Raw, "AttributeName")
			val := client.FieldValue(item.Raw, "CurrentValue")
			// SNMPAlert.N.Destination attributes
			if len(name) > 12 && name[:10] == "SNMPAlert." && len(name) > 13 &&
				name[len(name)-12:] == ".Destination" && val != "" && val != "0.0.0.0" {
				alertDests = append(alertDests, val)
			}
		}
	}
	state.AlertDestinations = strList(alertDests)

	// ---- Sessions ----
	if items, err := d.client.EnumerateAndPull(client.ResourceSessionView); err == nil {
		state.ActiveSessions = types.Int64Value(int64(len(items)))
	} else {
		state.ActiveSessions = types.Int64Value(0)
	}

	// ---- Licenses ----
	licenseDescs := []string{}
	if items, err := d.client.EnumerateAndPull(client.ResourceLicenseManageable); err == nil {
		for _, item := range items {
			if desc := client.FieldValue(item.Raw, "LicenseDescription"); desc != "" {
				licenseDescs = append(licenseDescs, desc)
			}
		}
	}
	state.Licenses = strList(licenseDescs)

	// ---- Intrusion ----
	state.IntrusionStatus = types.StringValue("NotApplicable")
	if items, err := d.client.EnumerateAndPull(client.ResourceIntrusionView); err == nil && len(items) > 0 {
		state.IntrusionStatus = types.StringValue(client.FieldValue(items[0].Raw, "IntrusionStatus"))
	}

	tflog.Info(ctx, "Discovery complete", map[string]interface{}{
		"service_tag":   state.ServiceTag.ValueString(),
		"controllers":   len(ctrlFQDDs),
		"physical_disks": len(pdFQDDs),
		"nics":          len(nicFQDDs),
		"virtual_disks": len(vdFQDDs),
		"firmware_items": len(fwMap),
		"bios_attrs":    len(biosMap),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
