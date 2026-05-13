# terraform-provider-idrac7

A custom Terraform provider for Dell iDRAC 7 servers. Uses WS-MAN (SOAP over HTTPS) — the only management protocol available on iDRAC 7. Redfish is not supported on this generation.

Tested against: Dell PowerEdge R420, iDRAC firmware 2.65.65.65.

---

## Requirements

- [Go](https://golang.org/) >= 1.21
- [Terraform](https://www.terraform.io/) >= 1.5.0
- macOS: `codesign` (included with Xcode Command Line Tools)

---

## Building and installing

```bash
make install
```

This builds the provider binary and copies it to `~/.terraform.d/plugins/registry.terraform.io/local/dell/idrac7/0.1.0/<os>_<arch>/`.

On **macOS** you must sign the binary after install to avoid Gatekeeper killing it:

```bash
codesign --sign - ~/.terraform.d/plugins/registry.terraform.io/local/dell/idrac7/0.1.0/darwin_arm64/terraform-provider-idrac7
```

---

## Terraform configuration

### `~/.terraformrc` — dev overrides

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/local/dell/idrac7" = "/Users/<you>/.terraform.d/plugins/registry.terraform.io/local/dell/idrac7/0.1.0/darwin_arm64"
  }
  direct {}
}
```

### `required_providers` block

```hcl
terraform {
  required_version = ">= 1.5.0"
  required_providers {
    idrac7 = {
      source  = "registry.terraform.io/local/dell/idrac7"
      version = "0.1.0"
    }
  }
}

provider "idrac7" {
  host         = "192.168.1.30"   # iDRAC IP or hostname
  username     = "root"
  password     = "calvin"
  ssl_insecure = true            # required for self-signed iDRAC certificates
}
```

---

## Data sources

| Data source | Description |
|---|---|
| `idrac7_discovery` | Full server snapshot — zero prior knowledge required. Returns service tag, model, all FQDDs, firmware map, BIOS attributes, sensor summary, licenses, intrusion status. |
| `idrac7_system_info` | System identity: model, service tag, BIOS version, power state, hostname, OS, RAM, CPU count. |
| `idrac7_hardware_inventory` | CPUs, DIMMs, NICs, RAID controllers, physical disks, virtual disks. |
| `idrac7_sensors` | Numeric sensors, fan speeds, PSU status. |
| `idrac7_firmware_inventory` | All installed firmware versions per component FQDD. |
| `idrac7_bios_all` | All current BIOS attributes (name → value map) plus full metadata (type, allowed values, read-only flag). |
| `idrac7_physical_disks` | Physical disk inventory with health status. |
| `idrac7_virtual_disks` | Virtual disk (RAID volume) configuration. |
| `idrac7_controllers` | RAID controller inventory. |
| `idrac7_batteries` | PERC/CMOS battery status. |
| `idrac7_fans` | Fan sensor data. |
| `idrac7_front_panel` | Front panel controller firmware. |
| `idrac7_removable_flash_media` | SD card / vFlash status. |
| `idrac7_enclosures` | Storage enclosure inventory. |
| `idrac7_host_os_network` | Host OS NIC info (requires iDRAC Service Module). |
| `idrac7_licenses` | Installed iDRAC feature licenses. |
| `idrac7_sessions` | Active iDRAC sessions. |
| `idrac7_intrusion` | Chassis intrusion detection status. |
| `idrac7_logs` | Lifecycle Controller and System Event Log entries. |

---

## Example

See [`examples/main.tf`](examples/main.tf) for a complete usage example.

### Minimal inventory read

```hcl
data "idrac7_discovery" "server" {}

output "snapshot" {
  value = data.idrac7_discovery.server
}
```

`terraform plan` will print the full server inventory without making any changes.

---

## idrac-report — standalone HTML inventory report

A standalone tool that connects to an iDRAC 7 and produces a self-contained HTML (or JSON) inventory report without requiring Terraform.

### Run

```bash
go run ./cmd/idrac-report/ \
  --host     192.168.1.30 \
  --user     root \
  --password calvin \
  --out      idrac-report.html
```

Or use environment variables:

```bash
export IDRAC_HOST=192.168.1.30
export IDRAC_USER=root
export IDRAC_PASSWORD=calvin

go run ./cmd/idrac-report/ --out idrac-report.html
```

### Flags

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--host` | `IDRAC_HOST` | _(required)_ | iDRAC hostname or IP address |
| `--user` | `IDRAC_USER` | `root` | iDRAC username |
| `--password` | `IDRAC_PASSWORD` | _(required)_ | iDRAC password |
| `--out` | — | `idrac-report.html` | Output file path. Use `.json` extension for JSON output |
| `--insecure` | — | `true` | Skip TLS certificate verification |

### Output

The report covers 19 sections across 5 groups:

| Group | Sections |
|---|---|
| Hardware | System, CPUs, Memory DIMMs, NICs, Fans, Power Supplies, Batteries, Numeric Sensors, Front Panel, Flash Media |
| Storage | RAID Controllers, Physical Disks, Virtual Disks, Enclosures |
| Firmware | Firmware Inventory |
| BIOS | _(via `idrac7_bios_all` data source in Terraform)_ |
| Management | Licenses, Active Sessions, Intrusion Detection, Lifecycle Logs |

Sections for WS-MAN classes unsupported on older firmware (e.g. fans and batteries on fw 2.65.65.65) display an error banner rather than failing the whole report.

> **Note:** iDRAC 7 cannot handle concurrent WS-MAN requests. Sections are fetched serially. Expect ~45–90 seconds for a full report.

### JSON output

```bash
go run ./cmd/idrac-report/ --host 192.168.1.30 --password calvin --out idrac-report.json
```

---

## Known limitations

- **iDRAC 7 only.** iDRAC 8/9/10 use Redfish; this provider will not work with them.
- **Serial WS-MAN requests.** Concurrent calls cause iDRAC 7 to return SOAP faults. All requests are serialised.
- **SOAP faults return HTTP 200.** The client inspects the body for `<s:Fault>` on every response.
- **Namespace-prefixed XML.** Go's `xml.Unmarshal` with `Items>*` silently returns 0 items when elements use namespace prefixes. A token-streaming parser is used instead.
- Some WS-MAN resource classes (`DCIM_Fan`, `DCIM_Battery`, `DCIM_LicenseManageable`, etc.) are not available on all firmware versions and will return a SOAP fault — these are treated as warnings, not errors.
