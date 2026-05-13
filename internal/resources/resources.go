// Package resources implements Terraform resources for iDRAC 7.
package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/steventaylor/terraform-provider-idrac7/internal/client"
)

// -----------------------------------------------------------------------
// Power Resource
// -----------------------------------------------------------------------

var _ resource.Resource = (*PowerResource)(nil)

// PowerResource manages server power state via WS-MAN CIM_ComputerSystem.
type PowerResource struct {
	client *client.Client
}

// NewPowerResource returns a new PowerResource factory.
func NewPowerResource() resource.Resource {
	return &PowerResource{}
}

func (r *PowerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_power"
}

func (r *PowerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

// powerModel maps the idrac7_power resource schema.
type powerModel struct {
	ID           types.String `tfsdk:"id"`
	DesiredState types.String `tfsdk:"desired_state"`
	CurrentState types.String `tfsdk:"current_state"`
	ForceReboot  types.Bool   `tfsdk:"force_reboot"`
}

// Power state constants (WS-MAN RequestedState values for CIM_ComputerSystem).
const (
	// RequestedState values
	powerStateOn               = "2"  // Power On
	powerStateOff              = "3"  // Power Off (graceful)
	powerStatePowerCycle       = "5"  // Power Cycle
	powerStateHardReset        = "10" // Reset (hard reboot)
	powerStateGracefulShutdown = "12" // Graceful Shutdown (ACPI)

	// Human-friendly aliases used in the schema
	desiredOn              = "on"
	desiredOff             = "off"
	desiredReboot          = "reboot"
	desiredGracefulReboot  = "graceful_reboot"
	desiredGracefulShutdown = "graceful_shutdown"
	desiredPowerCycle      = "power_cycle"
)

func desiredToRequestedState(desired string) (string, error) {
	switch strings.ToLower(desired) {
	case desiredOn:
		return powerStateOn, nil
	case desiredOff:
		return powerStateOff, nil
	case desiredReboot:
		return powerStateHardReset, nil
	case desiredGracefulReboot:
		return powerStateHardReset, nil // closest for iDRAC 7
	case desiredGracefulShutdown:
		return powerStateGracefulShutdown, nil
	case desiredPowerCycle:
		return powerStatePowerCycle, nil
	default:
		return "", fmt.Errorf("unknown desired_state %q; valid values: on, off, reboot, graceful_reboot, graceful_shutdown, power_cycle", desired)
	}
}

func (r *PowerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages the power state of a Dell iDRAC 7 server.

**desired_state** values:

| Value | Action |
|-------|--------|
| ` + "`on`" + ` | Power the server on |
| ` + "`off`" + ` | Graceful power off |
| ` + "`reboot`" + ` | Hard reset |
| ` + "`graceful_reboot`" + ` | Graceful OS reboot (maps to hard reset on iDRAC 7) |
| ` + "`graceful_shutdown`" + ` | Graceful OS shutdown via ACPI |
| ` + "`power_cycle`" + ` | Power cycle |
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier (host address).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"desired_state": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Desired power state: `on`, `off`, `reboot`, `graceful_reboot`, `graceful_shutdown`, `power_cycle`.",
			},
			"current_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current power state as reported by iDRAC.",
			},
			"force_reboot": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When `true`, issue the power action even if the server is already in the desired state.",
			},
		},
	}
}

// buildPowerInvoke creates the WS-MAN Invoke envelope for CIM_ComputerSystem.RequestStateChange.
func buildPowerInvoke(host, requestedState string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ComputerSystem">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ComputerSystem</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ComputerSystem/RequestStateChange</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="CreationClassName">CIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">srv:system</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT300S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:RequestStateChange_INPUT>
      <p:RequestedState>%s</p:RequestedState>
    </p:RequestStateChange_INPUT>
  </s:Body>
</s:Envelope>`, host, requestedState)
}

// getCurrentPowerState reads the current power state from DCIM_SystemView.
func (r *PowerResource) getCurrentPowerState(ctx context.Context) (string, error) {
	items, err := r.client.EnumerateAndPull(client.ResourceSystemView)
	if err != nil {
		return "", fmt.Errorf("reading system view: %w", err)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no system view instances returned")
	}
	return client.FieldValue(items[0].Raw, "PowerState"), nil
}

func (r *PowerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan powerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reqState, err := desiredToRequestedState(plan.DesiredState.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid desired_state", err.Error())
		return
	}

	tflog.Info(ctx, "Sending power state change", map[string]interface{}{
		"host": r.client.Host, "requested_state": reqState,
	})

	envelope := buildPowerInvoke(r.client.Host, reqState)
	_, err = r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("Power state change failed", err.Error())
		return
	}

	currentState, err := r.getCurrentPowerState(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read current power state", err.Error())
	}

	plan.ID = types.StringValue(r.client.Host)
	plan.CurrentState = types.StringValue(currentState)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PowerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state powerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	currentState, err := r.getCurrentPowerState(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read current power state", err.Error())
		return
	}
	state.CurrentState = types.StringValue(currentState)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PowerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan powerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reqState, err := desiredToRequestedState(plan.DesiredState.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid desired_state", err.Error())
		return
	}

	envelope := buildPowerInvoke(r.client.Host, reqState)
	_, err = r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("Power state change failed", err.Error())
		return
	}

	currentState, err := r.getCurrentPowerState(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read current power state after update", err.Error())
	}
	plan.CurrentState = types.StringValue(currentState)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete for power is a no-op — removing the resource from state does not power off the server.
func (r *PowerResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}

// -----------------------------------------------------------------------
// BIOS Attributes Resource
// -----------------------------------------------------------------------

var _ resource.Resource = (*BIOSAttributesResource)(nil)

// BIOSAttributesResource manages BIOS settings via DCIM_BIOSService.
type BIOSAttributesResource struct {
	client *client.Client
}

// NewBIOSAttributesResource returns a new BIOSAttributesResource factory.
func NewBIOSAttributesResource() resource.Resource {
	return &BIOSAttributesResource{}
}

func (r *BIOSAttributesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bios_attributes"
}

func (r *BIOSAttributesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

type biosAttributesModel struct {
	ID         types.String `tfsdk:"id"`
	Attributes types.Map    `tfsdk:"attributes"`
	// applied_attributes reflects what was actually committed (may differ if job pending)
	AppliedAttributes types.Map `tfsdk:"applied_attributes"`
}

func (r *BIOSAttributesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages BIOS attributes on a Dell iDRAC 7 server via ` + "`DCIM_BIOSService.SetAttributes`" + `.

BIOS attribute changes require a server reboot to take effect. This resource
creates a pending configuration job — the change will be applied at next boot.

**Note:** Only string and enumeration BIOS attributes are supported. Integer attributes
are accepted as string values.

## Example

~~~hcl
resource "idrac7_bios_attributes" "settings" {
  attributes = {
    "NumLock"          = "On"
    "ProcVirtualization" = "Enabled"
    "SysProfile"       = "PerfOptimized"
  }
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"attributes": schema.MapAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of BIOS attribute names to desired values.",
			},
			"applied_attributes": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of BIOS attribute names to their current committed values (read from iDRAC after apply).",
			},
		},
	}
}

// buildBIOSSetAttributes creates the WS-MAN Invoke for DCIM_BIOSService.SetAttributes.
func buildBIOSSetAttributes(host string, attrs map[string]string) string {
	attrXML := ""
	for k, v := range attrs {
		attrXML += fmt.Sprintf(`
      <p:AttributeName>%s</p:AttributeName>
      <p:AttributeValue>%s</p:AttributeValue>`, k, v)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_BIOSService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_BIOSService</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:Action s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_BIOSService/SetAttributes</a:Action>
    <wsman:SelectorSet>
      <wsman:Selector Name="CreationClassName">DCIM_BIOSService</wsman:Selector>
      <wsman:Selector Name="SystemCreationClassName">DCIM_ComputerSystem</wsman:Selector>
      <wsman:Selector Name="SystemName">DCIM:ComputerSystem</wsman:Selector>
      <wsman:Selector Name="Name">DCIM:BIOSService</wsman:Selector>
    </wsman:SelectorSet>
    <wsman:OperationTimeout>PT300S</wsman:OperationTimeout>
  </s:Header>
  <s:Body>
    <p:SetAttributes_INPUT>
      <p:Target>BIOS.Setup.1-1</p:Target>%s
    </p:SetAttributes_INPUT>
  </s:Body>
</s:Envelope>`, host, attrXML)
}

// readBIOSAttributes reads all current BIOS enumeration + string attributes.
func (r *BIOSAttributesResource) readBIOSAttributes(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string)

	for _, resClass := range []string{client.ResourceBIOSEnum, client.ResourceBIOSString, client.ResourceBIOSInteger} {
		items, err := r.client.EnumerateAndPull(resClass)
		if err != nil {
			tflog.Warn(ctx, "Could not enumerate BIOS resource class", map[string]interface{}{"class": resClass, "error": err.Error()})
			continue
		}
		for _, item := range items {
			name := client.FieldValue(item.Raw, "AttributeName")
			val := client.FieldValue(item.Raw, "CurrentValue")
			if name != "" {
				result[name] = val
			}
		}
	}
	return result, nil
}

func (r *BIOSAttributesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan biosAttributesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attrs := make(map[string]string)
	resp.Diagnostics.Append(plan.Attributes.ElementsAs(ctx, &attrs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	envelope := buildBIOSSetAttributes(r.client.Host, attrs)
	_, err := r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("BIOS SetAttributes failed", err.Error())
		return
	}

	// Read back applied values
	applied, err := r.readBIOSAttributes(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read back BIOS attributes", err.Error())
	}

	appliedMap, diags := types.MapValueFrom(ctx, types.StringType, applied)
	resp.Diagnostics.Append(diags...)

	plan.ID = types.StringValue(r.client.Host + "/bios")
	plan.AppliedAttributes = appliedMap
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BIOSAttributesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state biosAttributesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applied, err := r.readBIOSAttributes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read BIOS attributes", err.Error())
		return
	}
	appliedMap, diags := types.MapValueFrom(ctx, types.StringType, applied)
	resp.Diagnostics.Append(diags...)
	state.AppliedAttributes = appliedMap
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BIOSAttributesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan biosAttributesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attrs := make(map[string]string)
	resp.Diagnostics.Append(plan.Attributes.ElementsAs(ctx, &attrs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	envelope := buildBIOSSetAttributes(r.client.Host, attrs)
	_, err := r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("BIOS SetAttributes failed", err.Error())
		return
	}

	applied, _ := r.readBIOSAttributes(ctx)
	appliedMap, diags := types.MapValueFrom(ctx, types.StringType, applied)
	resp.Diagnostics.Append(diags...)
	plan.AppliedAttributes = appliedMap
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BIOSAttributesResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Removing this resource from state does not reset BIOS to defaults.
}

// -----------------------------------------------------------------------
// User Account Resource
// -----------------------------------------------------------------------

var _ resource.Resource = (*UserAccountResource)(nil)

// UserAccountResource manages iDRAC local user accounts via DCIM_iDRACCardAttribute.
type UserAccountResource struct {
	client *client.Client
}

// NewUserAccountResource returns a new UserAccountResource factory.
func NewUserAccountResource() resource.Resource {
	return &UserAccountResource{}
}

func (r *UserAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_account"
}

func (r *UserAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

// userAccountModel maps the idrac7_user_account resource schema.
// iDRAC 7 supports up to 16 local user accounts (slots 1-16; slot 1 = root).
type userAccountModel struct {
	ID          types.String `tfsdk:"id"`
	UserID      types.Int64  `tfsdk:"user_id"`       // 2-16
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Privilege   types.String `tfsdk:"privilege"`     // Administrator, Operator, ReadOnly, None
}

// iDRAC privilege bitmask values (used in DCIM_iDRACCardAttribute)
var privilegeMap = map[string]string{
	"Administrator": "511",
	"Operator":      "499",
	"ReadOnly":      "1",
	"None":          "0",
}

func (r *UserAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a local iDRAC 7 user account.

iDRAC 7 supports up to 16 local user accounts. Slot 1 is reserved for the root account.
Use ` + "`user_id`" + ` 2–16 for additional accounts.

## Example

~~~hcl
resource "idrac7_user_account" "ops" {
  user_id   = 3
  username  = "opsadmin"
  password  = "S3cur3P@ss!"
  enabled   = true
  privilege = "Operator"
}
~~~
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "iDRAC user slot (2–16). Slot 1 is reserved for root.",
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Username for the account.",
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for the account.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the account is enabled.",
			},
			"privilege": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Privilege level: `Administrator`, `Operator`, `ReadOnly`, `None`.",
			},
		},
	}
}

// buildUserAttributeSet creates a WS-MAN Invoke for DCIM_iDRACCardService.ApplyAttributes
// to set user account attributes.
func buildUserAttributeSet(host string, userID int64, username, password string, enabled bool, privilege string) string {
	enabledVal := "Disabled"
	if enabled {
		enabledVal = "Enabled"
	}
	privVal := privilegeMap[privilege]
	if privVal == "" {
		privVal = privilegeMap["ReadOnly"]
	}

	userPrefix := fmt.Sprintf("Users.%d", userID)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsman="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:p="http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService">
  <s:Header>
    <a:To>https://%s/wsman</a:To>
    <wsman:ResourceURI s:mustUnderstand="true">http://schemas.dell.com/wbem/wscim/1/cim-schema/2/DCIM_iDRACCardService</wsman:ResourceURI>
    <a:ReplyTo>
      <a:Address s:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
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
      <p:AttributeName>%s.UserName</p:AttributeName>
      <p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>%s.Password</p:AttributeName>
      <p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>%s.Enable</p:AttributeName>
      <p:AttributeValue>%s</p:AttributeValue>
      <p:AttributeName>%s.Privilege</p:AttributeName>
      <p:AttributeValue>%s</p:AttributeValue>
    </p:ApplyAttributes_INPUT>
  </s:Body>
</s:Envelope>`, host,
		userPrefix, username,
		userPrefix, password,
		userPrefix, enabledVal,
		userPrefix, privVal)
}

func (r *UserAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.UserID.ValueInt64() < 2 || plan.UserID.ValueInt64() > 16 {
		resp.Diagnostics.AddError("Invalid user_id", "user_id must be between 2 and 16 (slot 1 is reserved for root)")
		return
	}

	privilege := plan.Privilege.ValueString()
	if privilege == "" {
		privilege = "ReadOnly"
	}

	envelope := buildUserAttributeSet(
		r.client.Host,
		plan.UserID.ValueInt64(),
		plan.Username.ValueString(),
		plan.Password.ValueString(),
		plan.Enabled.ValueBool(),
		privilege,
	)

	tflog.Info(ctx, "Creating iDRAC user account", map[string]interface{}{
		"user_id": plan.UserID.ValueInt64(), "username": plan.Username.ValueString(),
	})

	_, err := r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user account", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/user/%d", r.client.Host, plan.UserID.ValueInt64()))
	if plan.Privilege.IsNull() || plan.Privilege.IsUnknown() {
		plan.Privilege = types.StringValue(privilege)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back the user attributes from DCIM_iDRACCardAttribute
	items, err := r.client.EnumerateAndPull(client.ResourceiDRACCard)
	if err != nil {
		resp.Diagnostics.AddError("Could not read iDRAC card attributes", err.Error())
		return
	}

	prefix := fmt.Sprintf("Users.%d.", state.UserID.ValueInt64())
	for _, item := range items {
		name := client.FieldValue(item.Raw, "AttributeName")
		val := client.FieldValue(item.Raw, "CurrentValue")

		switch {
		case name == prefix+"UserName":
			state.Username = types.StringValue(val)
		case name == prefix+"Enable":
			state.Enabled = types.BoolValue(strings.EqualFold(val, "enabled"))
		case name == prefix+"Privilege":
			// Convert bitmask back to label
			for label, mask := range privilegeMap {
				if val == mask {
					state.Privilege = types.StringValue(label)
					break
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	privilege := plan.Privilege.ValueString()
	if privilege == "" {
		privilege = "ReadOnly"
	}

	envelope := buildUserAttributeSet(
		r.client.Host,
		plan.UserID.ValueInt64(),
		plan.Username.ValueString(),
		plan.Password.ValueString(),
		plan.Enabled.ValueBool(),
		privilege,
	)

	_, err := r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update user account", err.Error())
		return
	}

	plan.Privilege = types.StringValue(privilege)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Disable and clear the user slot
	envelope := buildUserAttributeSet(
		r.client.Host,
		state.UserID.ValueInt64(),
		"",    // clear username
		"",    // clear password
		false, // disable
		"None",
	)

	tflog.Info(ctx, "Deleting iDRAC user account (disabling slot)", map[string]interface{}{
		"user_id": state.UserID.ValueInt64(),
	})

	_, err := r.client.PostRaw(envelope)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete user account", err.Error())
	}
}

// -----------------------------------------------------------------------
// Shared attr type for object lists (used in hardware inventory)
// -----------------------------------------------------------------------

// ObjectListAttrTypes is a helper so datasources can reference attr types.
var ObjectListAttrTypes = map[string]attr.Type{}
