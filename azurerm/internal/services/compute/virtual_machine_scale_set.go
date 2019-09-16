package compute

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

type VirtualMachineScaleSetResourceID struct {
	Base azure.ResourceID

	Name string
}

func ParseVirtualMachineScaleSetResourceID(input string) (*VirtualMachineScaleSetResourceID, error) {
	id, err := azure.ParseAzureResourceID(input)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] Unable to parse Virtual Machine Scale Set ID %q: %+v", input, err)
	}

	networkSecurityGroup := VirtualMachineScaleSetResourceID{
		Base: *id,
		Name: id.Path["virtualMachineScaleSets"],
	}

	if networkSecurityGroup.Name == "" {
		return nil, fmt.Errorf("ID was missing the `virtualMachineScaleSets` element")
	}

	return &networkSecurityGroup, nil
}

func VirtualMachineScaleSetAdditionalCapabilitiesSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ultra_ssd_enabled": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  false,
				},
			},
		},
	}
}

func ExpandVirtualMachineScaleSetAdditionalCapabilities(input []interface{}) *compute.AdditionalCapabilities {
	capabilities := compute.AdditionalCapabilities{}

	if len(input) > 0 {
		raw := input[0].(map[string]interface{})

		capabilities.UltraSSDEnabled = utils.Bool(raw["ultra_ssd_enabled"].(bool))
	}

	return &capabilities
}

func FlattenVirtualMachineScaleSetAdditionalCapabilities(input *compute.AdditionalCapabilities) []interface{} {
	if input == nil {
		return []interface{}{}
	}

	ultraSsdEnabled := false

	if input.UltraSSDEnabled != nil {
		ultraSsdEnabled = *input.UltraSSDEnabled
	}

	return []interface{}{
		map[string]interface{}{
			"ultra_ssd_enabled": ultraSsdEnabled,
		},
	}
}

func VirtualMachineScaleSetOSDiskSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Required: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"caching": {
					Type:     schema.TypeString,
					Required: true,
					ValidateFunc: validation.StringInSlice([]string{
						string(compute.CachingTypesNone),
						string(compute.CachingTypesReadOnly),
						string(compute.CachingTypesReadWrite),
					}, false),
				},
				"storage_account_type": {
					Type:     schema.TypeString,
					Required: true,
					ValidateFunc: validation.StringInSlice([]string{
						// note: OS Disks don't support Ultra SSDs
						string(compute.StorageAccountTypesPremiumLRS),
						string(compute.StorageAccountTypesStandardLRS),
						string(compute.StorageAccountTypesStandardSSDLRS),
					}, false),
				},

				"diff_disk_settings": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					// TODO: should this be ForceNew?
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"option": {
								Type:     schema.TypeString,
								Required: true,
								ValidateFunc: validation.StringInSlice([]string{
									string(compute.Local),
								}, false),
							},
						},
					},
				},

				"disk_size_gb": {
					Type:         schema.TypeInt,
					Optional:     true,
					ValidateFunc: validation.IntBetween(0, 1023),
					// TODO: should this be ForceNew?
				},

				"write_accelerator_enabled": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  false,
					// TODO: should this be ForceNew?
				},
			},
		},
	}
}

func ExpandVirtualMachineScaleSetOSDisk(input []interface{}, osType compute.OperatingSystemTypes) *compute.VirtualMachineScaleSetOSDisk {
	raw := input[0].(map[string]interface{})
	disk := compute.VirtualMachineScaleSetOSDisk{
		Caching: compute.CachingTypes(raw["caching"].(string)),
		ManagedDisk: &compute.VirtualMachineScaleSetManagedDiskParameters{
			StorageAccountType: compute.StorageAccountTypes(raw["storage_account_type"].(string)),
		},
		WriteAcceleratorEnabled: utils.Bool(raw["write_accelerator_enabled"].(bool)),

		// these have to be hard-coded so there's no point exposing them
		CreateOption: compute.DiskCreateOptionTypesFromImage,
		OsType:       osType,
	}

	if osDiskSize := raw["disk_size_gb"].(int); osDiskSize > 0 {
		disk.DiskSizeGB = utils.Int32(int32(osDiskSize))
	}

	if diffDiskSettingsRaw := raw["diff_disk_settings"].([]interface{}); len(diffDiskSettingsRaw) > 0 {
		diffDiskRaw := diffDiskSettingsRaw[0].(map[string]interface{})
		disk.DiffDiskSettings = &compute.DiffDiskSettings{
			Option: compute.DiffDiskOptions(diffDiskRaw["option"].(string)),
		}
	}

	return &disk
}

func FlattenVirtualMachineScaleSetOSDisk(input *compute.VirtualMachineScaleSetOSDisk) []interface{} {
	if input == nil {
		return []interface{}{}
	}

	diffDataSettings := make([]interface{}, 0)
	if input.DiffDiskSettings != nil {
		diffDataSettings = append(diffDataSettings, map[string]interface{}{
			"option": string(input.DiffDiskSettings.Option),
		})
	}

	diskSizeGb := 0
	if input.DiskSizeGB != nil && *input.DiskSizeGB != 0 {
		diskSizeGb = int(*input.DiskSizeGB)
	}

	var storageAccountType string
	if input.ManagedDisk != nil {
		storageAccountType = string(input.ManagedDisk.StorageAccountType)
	}

	writeAcceleratorEnabled := false
	if input.WriteAcceleratorEnabled != nil {
		writeAcceleratorEnabled = *input.WriteAcceleratorEnabled
	}
	return []interface{}{
		map[string]interface{}{
			"caching":                   string(input.Caching),
			"disk_size_gb":              diskSizeGb,
			"diff_data_settings":        diffDataSettings,
			"storage_account_type":      storageAccountType,
			"write_accelerator_enabled": writeAcceleratorEnabled,
		},
	}
}

func VirtualMachineScaleSetSourceImageReferenceSchema() *schema.Schema {
	// whilst originally I was hoping we could use the 'id' from `azurerm_platform_image' unfortunately Azure doesn't
	// like this as a value for the 'id' field:
	// Id /...../Versions/16.04.201909091 is not a valid resource reference."
	// as such the image is split into two fields (source_image_id and source_image_reference) to provide better validation
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"publisher": {
					Type:          schema.TypeString,
					Required:      true,
					ConflictsWith: []string{"source_image_id"},
				},
				"offer": {
					Type:          schema.TypeString,
					Required:      true,
					ConflictsWith: []string{"source_image_id"},
				},
				"sku": {
					Type:          schema.TypeString,
					Required:      true,
					ConflictsWith: []string{"source_image_id"},
				},
				"version": {
					Type:          schema.TypeString,
					Required:      true,
					ConflictsWith: []string{"source_image_id"},
				},
			},
		},
	}
}

func ExpandVirtualMachineScaleSetSourceImageReference(input []interface{}) *compute.ImageReference {
	if len(input) == 0 {
		return nil
	}

	raw := input[0].(map[string]interface{})
	return &compute.ImageReference{
		Publisher: utils.String(raw["publisher"].(string)),
		Offer:     utils.String(raw["offer"].(string)),
		Sku:       utils.String(raw["sku"].(string)),
		Version:   utils.String(raw["version"].(string)),
	}
}

func FlattenVirtualMachineScaleSetSourceImageReference(input *compute.ImageReference) []interface{} {
	// since the image id is pulled out as a separate field, if that's set we should return an empty block here
	if input == nil || input.ID != nil {
		return []interface{}{}
	}

	var publisher, offer, sku, version string

	if input.Publisher != nil {
		publisher = *input.Publisher
	}
	if input.Offer != nil {
		offer = *input.Offer
	}
	if input.Sku != nil {
		sku = *input.Sku
	}
	if input.Version != nil {
		version = *input.Version
	}

	return []interface{}{
		map[string]interface{}{
			"publisher": publisher,
			"offer":     offer,
			"sku":       sku,
			"version":   version,
		},
	}
}

func VirtualMachineScaleSetUpgradePolicySchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Required: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"mode": {
					Type:     schema.TypeString,
					Required: true,
					ValidateFunc: validation.StringInSlice([]string{
						string(compute.Automatic),
						string(compute.Manual),
						string(compute.Rolling),
					}, false),
				},

				"automatic_os_upgrade_policy": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							// TODO: should these be optional + defaulted?
							"disable_automatic_rollback": {
								Type:     schema.TypeBool,
								Required: true,
							},
							"enable_automatic_os_upgrade": {
								Type:     schema.TypeBool,
								Required: true,
							},
						},
					},
				},

				"rolling_upgrade_policy": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"max_batch_instance_percent": {
								Type:     schema.TypeInt,
								Required: true,
							},
							"max_unhealthy_instance_percent": {
								Type:     schema.TypeInt,
								Required: true,
							},
							"max_unhealthy_upgraded_instance_percent": {
								Type:     schema.TypeInt,
								Required: true,
							},
							"pause_time_between_batches": {
								Type:     schema.TypeString,
								Required: true,
							},
						},
					},
				},
			},
		},
	}
}

func ExpandVirtualMachineScaleSetUpgradePolicy(input []interface{}) (*compute.UpgradePolicy, error) {
	raw := input[0].(map[string]interface{})
	automaticPoliciesRaw := raw["automatic_os_upgrade_policy"].([]interface{})
	rollingPoliciesRaw := raw["rolling_upgrade_policy"].([]interface{})

	policy := compute.UpgradePolicy{
		Mode: compute.UpgradeMode(raw["mode"].(string)),
	}

	if len(automaticPoliciesRaw) > 0 {
		if policy.Mode != compute.Automatic {
			return nil, fmt.Errorf("A `automatic_os_upgrade_policy` block cannot be specified when `mode` is not set to `Automatic`")
		}

		automaticRaw := automaticPoliciesRaw[0].(map[string]interface{})
		policy.AutomaticOSUpgradePolicy = &compute.AutomaticOSUpgradePolicy{
			DisableAutomaticRollback: utils.Bool(automaticRaw["disable_automatic_rollback"].(bool)),
			EnableAutomaticOSUpgrade: utils.Bool(automaticRaw["enable_automatic_os_upgrade"].(bool)),
		}
	}

	if len(rollingPoliciesRaw) > 0 {
		if policy.Mode != compute.Rolling {
			return nil, fmt.Errorf("A `rolling_upgrade_policy` block cannot be specified when `mode` is not set to `Rolling`")
		}

		rollingRaw := rollingPoliciesRaw[0].(map[string]interface{})
		policy.RollingUpgradePolicy = &compute.RollingUpgradePolicy{
			MaxBatchInstancePercent:             utils.Int32(int32(rollingRaw["max_batch_instance_percent"].(int))),
			MaxUnhealthyInstancePercent:         utils.Int32(int32(rollingRaw["max_unhealthy_instance_percent"].(int))),
			MaxUnhealthyUpgradedInstancePercent: utils.Int32(int32(rollingRaw["max_unhealthy_upgraded_instance_percent"].(int))),
			PauseTimeBetweenBatches:             utils.String(rollingRaw["pause_time_between_batches"].(string)),
		}
	}

	if policy.Mode == compute.Automatic && policy.AutomaticOSUpgradePolicy == nil {
		return nil, fmt.Errorf("A `automatic_os_upgrade_policy` block must be specified when `mode` is set to `Automatic`")
	}

	if policy.Mode == compute.Rolling && policy.RollingUpgradePolicy == nil {
		return nil, fmt.Errorf("A `rolling_upgrade_policy` block must be specified when `mode` is set to `Rolling`")
	}

	return &policy, nil
}

func FlattenVirtualMachineScaleSetUpgradePolicy(input *compute.UpgradePolicy) []interface{} {
	if input == nil {
		return []interface{}{}
	}

	automaticOutput := make([]interface{}, 0)
	if policy := input.AutomaticOSUpgradePolicy; policy != nil {
		disableAutomaticRollback := false
		enableAutomaticOSUpgrade := false

		if policy.DisableAutomaticRollback != nil {
			disableAutomaticRollback = *policy.DisableAutomaticRollback
		}

		if policy.EnableAutomaticOSUpgrade != nil {
			enableAutomaticOSUpgrade = *policy.EnableAutomaticOSUpgrade
		}

		automaticOutput = append(automaticOutput, map[string]interface{}{
			"disable_automatic_rollback":  disableAutomaticRollback,
			"enable_automatic_os_upgrade": enableAutomaticOSUpgrade,
		})
	}

	rollingOutput := make([]interface{}, 0)
	if policy := input.RollingUpgradePolicy; policy != nil {
		maxBatchInstancePercent := 0
		maxUnhealthyInstancePercent := 0
		maxUnhealthyUpgradedInstancePercent := 0
		pauseTimeBetweenBatches := ""

		if policy.MaxBatchInstancePercent != nil {
			maxBatchInstancePercent = int(*policy.MaxBatchInstancePercent)
		}
		if policy.MaxUnhealthyInstancePercent != nil {
			maxUnhealthyInstancePercent = int(*policy.MaxUnhealthyInstancePercent)
		}
		if policy.MaxUnhealthyUpgradedInstancePercent != nil {
			maxUnhealthyUpgradedInstancePercent = int(*policy.MaxUnhealthyUpgradedInstancePercent)
		}
		if policy.PauseTimeBetweenBatches != nil {
			pauseTimeBetweenBatches = *policy.PauseTimeBetweenBatches
		}

		rollingOutput = append(rollingOutput, map[string]interface{}{
			"max_batch_instance_percent":              maxBatchInstancePercent,
			"max_unhealthy_instance_percent":          maxUnhealthyInstancePercent,
			"max_unhealthy_upgraded_instance_percent": maxUnhealthyUpgradedInstancePercent,
			"pause_time_between_batches":              pauseTimeBetweenBatches,
		})
	}

	return []interface{}{
		map[string]interface{}{
			"mode":                        string(input.Mode),
			"automatic_os_upgrade_policy": automaticOutput,
			"rolling_upgrade_policy":      rollingOutput,
		},
	}
}
