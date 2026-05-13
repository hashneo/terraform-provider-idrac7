# terraform-provider-idrac7

A Terraform provider for Dell iDRAC 7, targeting the WS-MAN / DCIM API.

iDRAC 7 ships on Dell PowerEdge Generation 12 servers (e.g. R420, R620, R720).
It does **not** support Redfish — this provider uses SOAP-over-HTTPS (WS-Management)
against the DCIM namespace instead.

---

## Resources

| Resource | Description |
|----------|-------------|
| `idrac7_power` | Manage server power state (on/off/reboot/power_cycle/graceful_shutdown) |
| `idrac7_bios_attributes` | Read and write BIOS attributes via `DCIM_BIOSService.SetAttributes` |
| `idrac7_user_account` | Manage iDRAC local user accounts (slots 2–16) |
| `idrac7_network_settings` | Configure iDRAC NIC (IP, gateway, DNS, VLAN) |
| `idrac7_alert_destination` | SNMP/email alert destinations |
| `idrac7_virtual_disk` | Create and delete RAID virtual disks via `DCIM_RAIDService` |
| `idrac7_server_profile` | Export/import Server Configuration Profile (SCP) via `DCIM_LCService` |
| `idrac7_firmware_update` | Push firmware updates via `DCIM_SoftwareInstallationService` |

## Data Sources

| Data Source | Description |
|-------------|-------------|
| `idrac7_discovery` | **Full server snapshot** — identity, all FQDDs, firmware, BIOS, sensors, licenses, sessions, intrusion. Zero prior knowledge required. |
| `idrac7_system_info` | Model, service tag, BIOS version, firmware, power state, hostname |
| `idrac7_hardware_inventory` | CPUs, DIMMs, NICs, storage controllers, physical disks |
| `idrac7_sensors` | Numeric sensors (fans, temperatures, voltages) and PSU status |
| `idrac7_batteries` | PERC RAID controller battery / capacitor units |
| `idrac7_fans` | Fan modules and their status |
| `idrac7_front_panel` | Front panel management controller view |
| `idrac7_removable_flash_media` | SD/vFlash removable media devices |
| `idrac7_virtual_disks` | Existing virtual disk (RAID volume) inventory |
| `idrac7_enclosures` | Storage enclosure/backplane view |
| `idrac7_host_os_network` | Host OS network interfaces (requires iDRAC Service Module) |
| `idrac7_logs` | Lifecycle Controller and SEL log entries |
| `idrac7_firmware_inventory` | All installed firmware versions per component (`DCIM_SoftwareIdentity`) |
| `idrac7_bios_all` | Complete BIOS attribute map (name→value) plus full metadata (type, allowed values, read-only) |
| `idrac7_intrusion` | Chassis intrusion detection status |
| `idrac7_licenses` | Installed iDRAC feature licenses |
| `idrac7_sessions` | Active iDRAC sessions |

---

## Build & Install

```bash
# From this directory:
make build    # produces ./terraform-provider-idrac7 binary
make install  # installs to ~/.terraform.d/plugins/registry.terraform.io/local/dell/idrac7/0.1.0/<os_arch>/
```

After installing, add to your `required_providers`:

```hcl
terraform {
  required_providers {
    idrac7 = {
      source  = "registry.terraform.io/local/dell/idrac7"
      version = "0.1.0"
    }
  }
}
```

---

## Provider Configuration

```hcl
provider "idrac7" {
  host         = "192.168.1.30"   # iDRAC 7 IP or hostname
  username     = "root"
  password     = "calvin"
  ssl_insecure = true             # recommended for lab self-signed certs
}
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| `host` | Yes | iDRAC hostname or IP |
| `username` | Yes | iDRAC username |
| `password` | Yes | iDRAC password |
| `port` | No | WS-MAN port (default 443) |
| `ssl_insecure` | No | Skip TLS verification (default false) |

---

## Example Usage

See [`examples/main.tf`](./examples/main.tf) for a complete working example.

```hcl
# Read system info
data "idrac7_system_info" "server" {}

output "service_tag" {
  value = data.idrac7_system_info.server.service_tag
}

# Ensure server is on
resource "idrac7_power" "server" {
  desired_state = "on"
}

# Tune BIOS for virtualisation
resource "idrac7_bios_attributes" "server" {
  attributes = {
    "ProcVirtualization" = "Enabled"
    "SysProfile"         = "PerfOptimized"
    "NumLock"            = "On"
  }
}

# Add an operator account
resource "idrac7_user_account" "ops" {
  user_id   = 3
  username  = "opsadmin"
  password  = "S3cur3P@ss!"
  enabled   = true
  privilege = "Operator"
}
```

---

## iDRAC 7 WS-MAN API Reference

| DCIM Class | Used by |
|------------|---------|
| `DCIM_SystemView` | `idrac7_system_info`, `idrac7_power` (current state) |
| `DCIM_CPUView` | `idrac7_hardware_inventory` |
| `DCIM_MemoryView` | `idrac7_hardware_inventory` |
| `DCIM_NICView` | `idrac7_hardware_inventory` |
| `DCIM_ControllerView` | `idrac7_hardware_inventory` |
| `DCIM_PhysicalDiskView` | `idrac7_hardware_inventory` |
| `DCIM_NumericSensor` | `idrac7_sensors` |
| `DCIM_PowerSupplyView` | `idrac7_sensors` |
| `DCIM_BIOSService` (SetAttributes) | `idrac7_bios_attributes` |
| `DCIM_BIOSEnumeration` / `DCIM_BIOSString` | `idrac7_bios_attributes` (read) |
| `DCIM_iDRACCardService` (ApplyAttributes) | `idrac7_user_account` |
| `CIM_ComputerSystem` (RequestStateChange) | `idrac7_power` |

---

## Multiple Servers

Use provider aliases for multiple iDRAC 7 hosts:

```hcl
provider "idrac7" {
  alias    = "r420_01"
  host     = "192.168.1.30"
  username = "root"
  password = "..."
  ssl_insecure = true
}

provider "idrac7" {
  alias    = "r420_02"
  host     = "192.168.1.31"
  username = "root"
  password = "..."
  ssl_insecure = true
}

data "idrac7_system_info" "r420_01" {
  provider = idrac7.r420_01
}

data "idrac7_system_info" "r420_02" {
  provider = idrac7.r420_02
}
```

---

## Development

```bash
make fmt    # format code
make vet    # run go vet
make test   # run unit tests
make build  # compile
make install # install locally
```
