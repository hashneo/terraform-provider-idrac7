// Package provider implements the Terraform provider for Dell iDRAC 7.
// It wires together the provider schema, resources, and data sources.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
	"github.com/steventaylor/terraform-provider-idrac7/internal/datasources"
	"github.com/steventaylor/terraform-provider-idrac7/internal/resources"
)

// Ensure iDRAC7Provider satisfies the provider.Provider interface.
var _ provider.Provider = (*iDRAC7Provider)(nil)

// iDRAC7Provider is the provider implementation.
type iDRAC7Provider struct {
	version string
}

// New returns a provider factory function used by the provider server.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &iDRAC7Provider{version: version}
	}
}

// providerModel maps the provider HCL schema to Go types.
type providerModel struct {
	Host        types.String `tfsdk:"host"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Port        types.Int64  `tfsdk:"port"`
	SSLInsecure types.Bool   `tfsdk:"ssl_insecure"`
}

// Metadata returns the provider type name and version.
func (p *iDRAC7Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "idrac7"
	resp.Version = p.version
}

// Schema defines the provider-level configuration block.
func (p *iDRAC7Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
The **idrac7** provider manages Dell PowerEdge servers via the iDRAC 7 WS-MAN API.

iDRAC 7 uses SOAP-over-HTTPS (WS-Management) rather than Redfish, which was
only introduced on iDRAC 8 firmware 2.40+. This provider targets the Dell DCIM
WS-MAN interface natively.

## Example Usage

~~~hcl
provider "idrac7" {
  host        = "192.168.1.30"
  username    = "root"
  password    = "calvin"
  ssl_insecure = true
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Hostname or IP address of the iDRAC 7 interface.",
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "iDRAC username (default: `root`).",
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "iDRAC password.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "WS-MAN HTTPS port (default: `443`).",
			},
			"ssl_insecure": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip TLS certificate verification. Set `true` for self-signed iDRAC certificates.",
			},
		},
	}
}

// Configure builds the iDRAC WS-MAN client and stores it for use by resources and data sources.
func (p *iDRAC7Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host := config.Host.ValueString()
	username := config.Username.ValueString()
	password := config.Password.ValueString()
	sslInsecure := true
	if !config.SSLInsecure.IsNull() && !config.SSLInsecure.IsUnknown() {
		sslInsecure = config.SSLInsecure.ValueBool()
	}

	c := client.New(host, username, password, sslInsecure)

	// Make the client available to resources and data sources via Configure.
	resp.DataSourceData = c
	resp.ResourceData = c
}

// Resources returns the list of resources provided by this provider.
func (p *iDRAC7Provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Power & lifecycle
		resources.NewPowerResource,
		// BIOS
		resources.NewBIOSAttributesResource,
		// iDRAC user management
		resources.NewUserAccountResource,
		// iDRAC network settings
		resources.NewNetworkSettingsResource,
		// Alerts
		resources.NewAlertDestinationResource,
		// Storage — RAID virtual disks
		resources.NewVirtualDiskResource,
		// Server Configuration Profile (backup/export)
		resources.NewServerProfileResource,
		// Firmware updates
		resources.NewFirmwareUpdateResource,
	}
}

// DataSources returns the list of data sources provided by this provider.
func (p *iDRAC7Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// System overview
		datasources.NewSystemInfoDataSource,
		// Hardware inventory
		datasources.NewHardwareInventoryDataSource,
		datasources.NewBatteriesDataSource,
		datasources.NewFansDataSource,
		datasources.NewFrontPanelDataSource,
		datasources.NewRemovableFlashMediaDataSource,
		// Storage
		datasources.NewVirtualDisksDataSource,
		datasources.NewEnclosuresDataSource,
		// Sensors & power
		datasources.NewSensorsDataSource,
		// Host OS
		datasources.NewHostOSNetworkDataSource,
		// Logs
		datasources.NewLogsDataSource,
		// Firmware inventory
		datasources.NewFirmwareInventoryDataSource,
		// BIOS — full attribute dump
		datasources.NewBIOSAllDataSource,
		// Chassis intrusion
		datasources.NewIntrusionDataSource,
		// iDRAC licenses
		datasources.NewLicensesDataSource,
		// Active sessions
		datasources.NewSessionsDataSource,
		// Full server discovery (zero prior knowledge)
		datasources.NewDiscoveryDataSource,
	}
}
