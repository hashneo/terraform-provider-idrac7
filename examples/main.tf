# ============================================================
# Example usage of the terraform-provider-idrac7
# pointing at a Dell PowerEdge R420 with iDRAC 7
# ============================================================

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    idrac7 = {
      source  = "registry.terraform.io/local/dell/idrac7"
      version = "0.1.0"
    }
  }
}

# ------------------------------------------------------------
# Provider configuration
# ------------------------------------------------------------
provider "idrac7" {
  host         = "192.168.1.30"       # iDRAC 7 IP or hostname
  username     = "root"
  password     = "calvin"             # default iDRAC password — change this!
  ssl_insecure = true                 # self-signed cert on most lab iDRACs
}

# ------------------------------------------------------------
# Data Sources — read-only inventory
# ------------------------------------------------------------

# System overview: model, service tag, BIOS version, power state
data "idrac7_system_info" "r420" {}

output "server_model" {
  value = data.idrac7_system_info.r420.model
}
output "service_tag" {
  value = data.idrac7_system_info.r420.service_tag
}
output "bios_version" {
  value = data.idrac7_system_info.r420.bios_version
}
output "power_state" {
  value = data.idrac7_system_info.r420.power_state
}

# Full hardware inventory
data "idrac7_hardware_inventory" "r420" {}

output "cpus" {
  value = data.idrac7_hardware_inventory.r420.cpus
}
output "dimms" {
  value = data.idrac7_hardware_inventory.r420.dimms
}
output "nics" {
  value = data.idrac7_hardware_inventory.r420.nics
}
output "storage_controllers" {
  value = data.idrac7_hardware_inventory.r420.controllers
}
output "physical_disks" {
  value = data.idrac7_hardware_inventory.r420.physical_disks
}

# Sensor readings (fans, temperatures, PSUs)
data "idrac7_sensors" "r420" {}

output "sensors" {
  value = data.idrac7_sensors.r420.sensors
}
output "power_supplies" {
  value = data.idrac7_sensors.r420.power_supplies
}

# ------------------------------------------------------------
# Resources — manage server state
# ------------------------------------------------------------

# Ensure the server is powered on
resource "idrac7_power" "r420" {
  desired_state = "on"
}

# Set BIOS attributes
resource "idrac7_bios_attributes" "r420" {
  attributes = {
    "NumLock"              = "On"
    "ProcVirtualization"   = "Enabled"
    "SysProfile"           = "PerfOptimized"
    "EmbNic1"              = "Enabled"
    "HddSeq"               = "RAID"
  }
}

# Create an additional iDRAC user account
resource "idrac7_user_account" "opsadmin" {
  user_id   = 3
  username  = "opsadmin"
  password  = "Ch@ngeMe123!"
  enabled   = true
  privilege = "Operator"
}
