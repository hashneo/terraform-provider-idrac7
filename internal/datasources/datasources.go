// Package datasources implements Terraform data sources for iDRAC 7.
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

// Ensure SystemInfoDataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = (*SystemInfoDataSource)(nil)

// SystemInfoDataSource reads the DCIM_SystemView resource from iDRAC 7.
type SystemInfoDataSource struct {
	client *client.Client
}

// NewSystemInfoDataSource returns a new SystemInfoDataSource factory.
func NewSystemInfoDataSource() datasource.DataSource {
	return &SystemInfoDataSource{}
}

// Metadata returns the data source type name.
func (d *SystemInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_info"
}

// Configure stores the provider client.
func (d *SystemInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

// systemInfoModel maps DCIM_SystemView fields.
type systemInfoModel struct {
	ID              types.String `tfsdk:"id"`
	Model           types.String `tfsdk:"model"`
	Manufacturer    types.String `tfsdk:"manufacturer"`
	ServiceTag      types.String `tfsdk:"service_tag"`
	BIOSVersion     types.String `tfsdk:"bios_version"`
	OSName          types.String `tfsdk:"os_name"`
	OSVersion       types.String `tfsdk:"os_version"`
	MemoryTotalMB   types.Int64  `tfsdk:"memory_total_mb"`
	CPUCount        types.Int64  `tfsdk:"cpu_count"`
	IDRACFirmware   types.String `tfsdk:"idrac_firmware_version"`
	PowerState      types.String `tfsdk:"power_state"`
	HostName        types.String `tfsdk:"hostname"`
}

// Schema defines the data source schema.
func (d *SystemInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads system information from a Dell iDRAC 7 via `DCIM_SystemView`.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Service tag used as unique identifier."},
			"model":                schema.StringAttribute{Computed: true, MarkdownDescription: "Server model string (e.g. PowerEdge R420)."},
			"manufacturer":         schema.StringAttribute{Computed: true, MarkdownDescription: "Server manufacturer."},
			"service_tag":          schema.StringAttribute{Computed: true, MarkdownDescription: "Dell service tag."},
			"bios_version":         schema.StringAttribute{Computed: true, MarkdownDescription: "Current BIOS version string."},
			"os_name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Operating system name (if reported by iDRAC)."},
			"os_version":           schema.StringAttribute{Computed: true, MarkdownDescription: "Operating system version."},
			"memory_total_mb":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Total installed memory in MB."},
			"cpu_count":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of physical CPUs."},
			"idrac_firmware_version": schema.StringAttribute{Computed: true, MarkdownDescription: "iDRAC firmware version."},
			"power_state":          schema.StringAttribute{Computed: true, MarkdownDescription: "Current server power state (On/Off)."},
			"hostname":             schema.StringAttribute{Computed: true, MarkdownDescription: "Server hostname as reported by iDRAC."},
		},
	}
}

// Read fetches system info via WS-MAN Enumerate on DCIM_SystemView.
func (d *SystemInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state systemInfoModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading iDRAC 7 system info", map[string]interface{}{"host": d.client.Host})

	items, err := d.client.EnumerateAndPull(client.ResourceSystemView)
	if err != nil {
		resp.Diagnostics.AddError("WS-MAN enumerate failed", err.Error())
		return
	}
	if len(items) == 0 {
		resp.Diagnostics.AddError("No system info returned", "DCIM_SystemView returned no instances")
		return
	}

	raw := items[0].Raw

	parseInt64 := func(s string) types.Int64 {
		if s == "" {
			return types.Int64Value(0)
		}
		var n int64
		fmt.Sscanf(s, "%d", &n)
		return types.Int64Value(n)
	}

	svcTag := client.FieldValue(raw, "ServiceTag")
	state = systemInfoModel{
		ID:            types.StringValue(svcTag),
		Model:         types.StringValue(client.FieldValue(raw, "Model")),
		Manufacturer:  types.StringValue(client.FieldValue(raw, "Manufacturer")),
		ServiceTag:    types.StringValue(svcTag),
		BIOSVersion:   types.StringValue(client.FieldValue(raw, "BIOSVersionString")),
		OSName:        types.StringValue(client.FieldValue(raw, "OSName")),
		OSVersion:     types.StringValue(client.FieldValue(raw, "OSVersion")),
		MemoryTotalMB: parseInt64(client.FieldValue(raw, "SysMemTotalSize")),
		CPUCount:      parseInt64(client.FieldValue(raw, "CPUSocketsPopulated")),
		IDRACFirmware: types.StringValue(client.FieldValue(raw, "LifecycleControllerVersion")),
		PowerState:    types.StringValue(client.FieldValue(raw, "PowerState")),
		HostName:      types.StringValue(client.FieldValue(raw, "HostName")),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -----------------------------------------------------------------------
// HardwareInventoryDataSource
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*HardwareInventoryDataSource)(nil)

// HardwareInventoryDataSource enumerates CPUs, DIMMs, NICs, controllers, and disks.
type HardwareInventoryDataSource struct {
	client *client.Client
}

// NewHardwareInventoryDataSource returns a new HardwareInventoryDataSource factory.
func NewHardwareInventoryDataSource() datasource.DataSource {
	return &HardwareInventoryDataSource{}
}

func (d *HardwareInventoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hardware_inventory"
}

func (d *HardwareInventoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

type hardwareInventoryModel struct {
	ID          types.String `tfsdk:"id"`
	CPUs        types.List   `tfsdk:"cpus"`
	DIMMs       types.List   `tfsdk:"dimms"`
	NICs        types.List   `tfsdk:"nics"`
	Controllers types.List   `tfsdk:"controllers"`
	PhysDisks   types.List   `tfsdk:"physical_disks"`
}

type cpuItem struct {
	FQDD         types.String `tfsdk:"fqdd"`
	Model        types.String `tfsdk:"model"`
	Manufacturer types.String `tfsdk:"manufacturer"`
	MaxSpeed     types.String `tfsdk:"max_speed_mhz"`
	Cores        types.String `tfsdk:"cores"`
	Threads      types.String `tfsdk:"threads"`
}

type dimmItem struct {
	FQDD         types.String `tfsdk:"fqdd"`
	Model        types.String `tfsdk:"model"`
	Manufacturer types.String `tfsdk:"manufacturer"`
	SizeMB       types.String `tfsdk:"size_mb"`
	Speed        types.String `tfsdk:"speed_mhz"`
	MemType      types.String `tfsdk:"memory_type"`
}

type nicItem struct {
	FQDD       types.String `tfsdk:"fqdd"`
	ProductName types.String `tfsdk:"product_name"`
	MACAddress  types.String `tfsdk:"mac_address"`
	LinkSpeed   types.String `tfsdk:"link_speed"`
}

type controllerItem struct {
	FQDD        types.String `tfsdk:"fqdd"`
	ProductName types.String `tfsdk:"product_name"`
	FWVersion   types.String `tfsdk:"firmware_version"`
	RaidTypes   types.String `tfsdk:"supported_raid_levels"`
}

type physDiskItem struct {
	FQDD       types.String `tfsdk:"fqdd"`
	Model      types.String `tfsdk:"model"`
	SerialNum  types.String `tfsdk:"serial_number"`
	SizeBytes  types.String `tfsdk:"size_bytes"`
	MediaType  types.String `tfsdk:"media_type"`
	RaidStatus types.String `tfsdk:"raid_status"`
}

var cpuAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "model": types.StringType,
	"manufacturer": types.StringType, "max_speed_mhz": types.StringType,
	"cores": types.StringType, "threads": types.StringType,
}
var dimmAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "model": types.StringType,
	"manufacturer": types.StringType, "size_mb": types.StringType,
	"speed_mhz": types.StringType, "memory_type": types.StringType,
}
var nicAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "product_name": types.StringType,
	"mac_address": types.StringType, "link_speed": types.StringType,
}
var controllerAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "product_name": types.StringType,
	"firmware_version": types.StringType, "supported_raid_levels": types.StringType,
}
var physDiskAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "model": types.StringType,
	"serial_number": types.StringType, "size_bytes": types.StringType,
	"media_type": types.StringType, "raid_status": types.StringType,
}

func (d *HardwareInventoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads full hardware inventory from iDRAC 7 (CPUs, DIMMs, NICs, storage controllers, physical disks).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"cpus": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":          schema.StringAttribute{Computed: true},
						"model":         schema.StringAttribute{Computed: true},
						"manufacturer":  schema.StringAttribute{Computed: true},
						"max_speed_mhz": schema.StringAttribute{Computed: true},
						"cores":         schema.StringAttribute{Computed: true},
						"threads":       schema.StringAttribute{Computed: true},
					},
				},
			},
			"dimms": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":         schema.StringAttribute{Computed: true},
						"model":        schema.StringAttribute{Computed: true},
						"manufacturer": schema.StringAttribute{Computed: true},
						"size_mb":      schema.StringAttribute{Computed: true},
						"speed_mhz":    schema.StringAttribute{Computed: true},
						"memory_type":  schema.StringAttribute{Computed: true},
					},
				},
			},
			"nics": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":         schema.StringAttribute{Computed: true},
						"product_name": schema.StringAttribute{Computed: true},
						"mac_address":  schema.StringAttribute{Computed: true},
						"link_speed":   schema.StringAttribute{Computed: true},
					},
				},
			},
			"controllers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":                  schema.StringAttribute{Computed: true},
						"product_name":          schema.StringAttribute{Computed: true},
						"firmware_version":      schema.StringAttribute{Computed: true},
						"supported_raid_levels": schema.StringAttribute{Computed: true},
					},
				},
			},
			"physical_disks": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":          schema.StringAttribute{Computed: true},
						"model":         schema.StringAttribute{Computed: true},
						"serial_number": schema.StringAttribute{Computed: true},
						"size_bytes":    schema.StringAttribute{Computed: true},
						"media_type":    schema.StringAttribute{Computed: true},
						"raid_status":   schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *HardwareInventoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state hardwareInventoryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(d.client.Host)

	// CPUs
	cpuItems, err := d.client.EnumerateAndPull(client.ResourceCPUView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read CPU inventory", err.Error())
	}
	cpuObjs := make([]attr.Value, 0, len(cpuItems))
	for _, item := range cpuItems {
		r := item.Raw
		obj, diags := types.ObjectValue(cpuAttrTypes, map[string]attr.Value{
			"fqdd":          types.StringValue(client.FieldValue(r, "FQDD")),
			"model":         types.StringValue(client.FieldValue(r, "Model")),
			"manufacturer":  types.StringValue(client.FieldValue(r, "Manufacturer")),
			"max_speed_mhz": types.StringValue(client.FieldValue(r, "MaxClockSpeed")),
			"cores":         types.StringValue(client.FieldValue(r, "NumberOfProcessorCores")),
			"threads":       types.StringValue(client.FieldValue(r, "NumberOfEnabledThreads")),
		})
		resp.Diagnostics.Append(diags...)
		cpuObjs = append(cpuObjs, obj)
	}
	state.CPUs, _ = types.ListValue(types.ObjectType{AttrTypes: cpuAttrTypes}, cpuObjs)

	// DIMMs
	dimmItems, err := d.client.EnumerateAndPull(client.ResourceMemoryView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read DIMM inventory", err.Error())
	}
	dimmObjs := make([]attr.Value, 0, len(dimmItems))
	for _, item := range dimmItems {
		r := item.Raw
		obj, diags := types.ObjectValue(dimmAttrTypes, map[string]attr.Value{
			"fqdd":         types.StringValue(client.FieldValue(r, "FQDD")),
			"model":        types.StringValue(client.FieldValue(r, "Model")),
			"manufacturer": types.StringValue(client.FieldValue(r, "Manufacturer")),
			"size_mb":      types.StringValue(client.FieldValue(r, "Size")),
			"speed_mhz":    types.StringValue(client.FieldValue(r, "Speed")),
			"memory_type":  types.StringValue(client.FieldValue(r, "MemoryType")),
		})
		resp.Diagnostics.Append(diags...)
		dimmObjs = append(dimmObjs, obj)
	}
	state.DIMMs, _ = types.ListValue(types.ObjectType{AttrTypes: dimmAttrTypes}, dimmObjs)

	// NICs
	nicItems, err := d.client.EnumerateAndPull(client.ResourceNICView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read NIC inventory", err.Error())
	}
	nicObjs := make([]attr.Value, 0, len(nicItems))
	for _, item := range nicItems {
		r := item.Raw
		obj, diags := types.ObjectValue(nicAttrTypes, map[string]attr.Value{
			"fqdd":         types.StringValue(client.FieldValue(r, "FQDD")),
			"product_name": types.StringValue(client.FieldValue(r, "ProductName")),
			"mac_address":  types.StringValue(client.FieldValue(r, "CurrentMACAddress")),
			"link_speed":   types.StringValue(client.FieldValue(r, "LinkSpeed")),
		})
		resp.Diagnostics.Append(diags...)
		nicObjs = append(nicObjs, obj)
	}
	state.NICs, _ = types.ListValue(types.ObjectType{AttrTypes: nicAttrTypes}, nicObjs)

	// Storage Controllers
	ctrlItems, err := d.client.EnumerateAndPull(client.ResourceControllerView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read controller inventory", err.Error())
	}
	ctrlObjs := make([]attr.Value, 0, len(ctrlItems))
	for _, item := range ctrlItems {
		r := item.Raw
		obj, diags := types.ObjectValue(controllerAttrTypes, map[string]attr.Value{
			"fqdd":                  types.StringValue(client.FieldValue(r, "FQDD")),
			"product_name":          types.StringValue(client.FieldValue(r, "ProductName")),
			"firmware_version":      types.StringValue(client.FieldValue(r, "ControllerFirmwareVersion")),
			"supported_raid_levels": types.StringValue(client.FieldValue(r, "SupportedInitializationTypes")),
		})
		resp.Diagnostics.Append(diags...)
		ctrlObjs = append(ctrlObjs, obj)
	}
	state.Controllers, _ = types.ListValue(types.ObjectType{AttrTypes: controllerAttrTypes}, ctrlObjs)

	// Physical Disks
	diskItems, err := d.client.EnumerateAndPull(client.ResourcePhysDiskView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read physical disk inventory", err.Error())
	}
	diskObjs := make([]attr.Value, 0, len(diskItems))
	for _, item := range diskItems {
		r := item.Raw
		obj, diags := types.ObjectValue(physDiskAttrTypes, map[string]attr.Value{
			"fqdd":          types.StringValue(client.FieldValue(r, "FQDD")),
			"model":         types.StringValue(client.FieldValue(r, "Model")),
			"serial_number": types.StringValue(client.FieldValue(r, "SerialNumber")),
			"size_bytes":    types.StringValue(client.FieldValue(r, "SizeInBytes")),
			"media_type":    types.StringValue(client.FieldValue(r, "MediaType")),
			"raid_status":   types.StringValue(client.FieldValue(r, "RaidStatus")),
		})
		resp.Diagnostics.Append(diags...)
		diskObjs = append(diskObjs, obj)
	}
	state.PhysDisks, _ = types.ListValue(types.ObjectType{AttrTypes: physDiskAttrTypes}, diskObjs)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -----------------------------------------------------------------------
// SensorsDataSource
// -----------------------------------------------------------------------

var _ datasource.DataSource = (*SensorsDataSource)(nil)

// SensorsDataSource reads numeric sensors (fans, temps) and PSU status.
type SensorsDataSource struct {
	client *client.Client
}

// NewSensorsDataSource returns a new SensorsDataSource factory.
func NewSensorsDataSource() datasource.DataSource {
	return &SensorsDataSource{}
}

func (d *SensorsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensors"
}

func (d *SensorsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

type sensorsModel struct {
	ID      types.String `tfsdk:"id"`
	Sensors types.List   `tfsdk:"sensors"`
	PSUs    types.List   `tfsdk:"power_supplies"`
}

type sensorItem struct {
	Name            types.String `tfsdk:"name"`
	SensorType      types.String `tfsdk:"sensor_type"`
	CurrentReading  types.String `tfsdk:"current_reading"`
	BaseUnits       types.String `tfsdk:"base_units"`
	UpperWarning    types.String `tfsdk:"upper_threshold_warning"`
	UpperCritical   types.String `tfsdk:"upper_threshold_critical"`
	State           types.String `tfsdk:"state"`
}

type psuItem struct {
	FQDD           types.String `tfsdk:"fqdd"`
	ProductName    types.String `tfsdk:"product_name"`
	InputWatts     types.String `tfsdk:"input_watts"`
	OutputWatts    types.String `tfsdk:"output_watts"`
	PrimaryStatus  types.String `tfsdk:"primary_status"`
}

var sensorAttrTypes = map[string]attr.Type{
	"name": types.StringType, "sensor_type": types.StringType,
	"current_reading": types.StringType, "base_units": types.StringType,
	"upper_threshold_warning": types.StringType, "upper_threshold_critical": types.StringType,
	"state": types.StringType,
}
var psuAttrTypes = map[string]attr.Type{
	"fqdd": types.StringType, "product_name": types.StringType,
	"input_watts": types.StringType, "output_watts": types.StringType,
	"primary_status": types.StringType,
}

func (d *SensorsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads numeric sensor data (fans, temperatures) and power supply status from iDRAC 7.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"sensors": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of numeric sensors (fans, temperatures, voltages).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":                    schema.StringAttribute{Computed: true},
						"sensor_type":             schema.StringAttribute{Computed: true},
						"current_reading":         schema.StringAttribute{Computed: true},
						"base_units":              schema.StringAttribute{Computed: true},
						"upper_threshold_warning":  schema.StringAttribute{Computed: true},
						"upper_threshold_critical": schema.StringAttribute{Computed: true},
						"state":                   schema.StringAttribute{Computed: true},
					},
				},
			},
			"power_supplies": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of power supply units.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdd":           schema.StringAttribute{Computed: true},
						"product_name":   schema.StringAttribute{Computed: true},
						"input_watts":    schema.StringAttribute{Computed: true},
						"output_watts":   schema.StringAttribute{Computed: true},
						"primary_status": schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *SensorsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state sensorsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(d.client.Host)

	// Numeric sensors (temperatures, fans, voltages)
	sensorItems, err := d.client.EnumerateAndPull(client.ResourceNumericSensor)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read sensor data", err.Error())
	}
	sensorObjs := make([]attr.Value, 0, len(sensorItems))
	for _, item := range sensorItems {
		r := item.Raw
		obj, diags := types.ObjectValue(sensorAttrTypes, map[string]attr.Value{
			"name":                    types.StringValue(client.FieldValue(r, "ElementName")),
			"sensor_type":             types.StringValue(client.FieldValue(r, "SensorType")),
			"current_reading":         types.StringValue(client.FieldValue(r, "CurrentReading")),
			"base_units":              types.StringValue(client.FieldValue(r, "BaseUnits")),
			"upper_threshold_warning":  types.StringValue(client.FieldValue(r, "UpperThresholdNonCritical")),
			"upper_threshold_critical": types.StringValue(client.FieldValue(r, "UpperThresholdCritical")),
			"state":                   types.StringValue(client.FieldValue(r, "HealthState")),
		})
		resp.Diagnostics.Append(diags...)
		sensorObjs = append(sensorObjs, obj)
	}
	state.Sensors, _ = types.ListValue(types.ObjectType{AttrTypes: sensorAttrTypes}, sensorObjs)

	// Power supplies
	psuItems, err := d.client.EnumerateAndPull(client.ResourcePSView)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read PSU data", err.Error())
	}
	psuObjs := make([]attr.Value, 0, len(psuItems))
	for _, item := range psuItems {
		r := item.Raw
		obj, diags := types.ObjectValue(psuAttrTypes, map[string]attr.Value{
			"fqdd":           types.StringValue(client.FieldValue(r, "FQDD")),
			"product_name":   types.StringValue(client.FieldValue(r, "ProductName")),
			"input_watts":    types.StringValue(client.FieldValue(r, "InputWatts")),
			"output_watts":   types.StringValue(client.FieldValue(r, "OutputWatts")),
			"primary_status": types.StringValue(client.FieldValue(r, "PrimaryStatus")),
		})
		resp.Diagnostics.Append(diags...)
		psuObjs = append(psuObjs, obj)
	}
	state.PSUs, _ = types.ListValue(types.ObjectType{AttrTypes: psuAttrTypes}, psuObjs)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
