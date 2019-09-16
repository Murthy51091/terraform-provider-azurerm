package azurerm

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	computeSvc "github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/compute"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmLinuxVirtualMachineScaleSet() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmLinuxVirtualMachineScaleSetCreate,
		Read:   resourceArmLinuxVirtualMachineScaleSetRead,
		Update: resourceArmLinuxVirtualMachineScaleSetUpdate,
		Delete: resourceArmLinuxVirtualMachineScaleSetDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				// The value must not be empty.
				// The value can only contain alphanumeric characters and cannot start with a number.
				// Azure resource names cannot contain special characters \/""[]:|<>+=;,?*@& or begin with '_' or end with '.' or '-'
				// The value must be between 1 and 64 characters long.
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"location": azure.SchemaLocation(),

			// Required
			"admin_username": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.NoEmptyStrings,
			},

			"os_disk": computeSvc.VirtualMachineScaleSetOSDiskSchema(),

			// Optional
			"additional_capabilities": computeSvc.VirtualMachineScaleSetAdditionalCapabilitiesSchema(),

			"admin_password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},

			"admin_ssh_key": computeSvc.SSHKeysSchema(),

			"computer_name_prefix": {
				// TODO: could we default this from the VMSS name, perhaps?
				Type:     schema.TypeString,
				Required: true,
				// TODO: Computer name prefixes must be 1 to 15 characters long.
				ValidateFunc: validate.NoEmptyStrings,
			},

			"disable_password_authentication": {
				Type:     schema.TypeBool,
				Required: true,
				ForceNew: true, // TODO: this needs checking
			},

			"do_not_run_extensions_on_overprovisioned_machines": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"eviction_policy": {
				// only applicable when `priority` is set to `Low`
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.Deallocate),
					string(compute.Delete),
				}, false),
			},

			"overprovision": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"platform_fault_domain_count": {
				Type:     schema.TypeInt,
				Optional: true,
			},

			"priority": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  string(compute.Regular),
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.Low),
					string(compute.Regular),
				}, false),
			},

			"provision_vm_agent": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				// TODO: check this default
			},

			"proximity_placement_group_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"single_placement_group": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"source_image_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"source_image_reference": computeSvc.VirtualMachineScaleSetSourceImageReferenceSchema(),

			// TODO: find & set me
			// should this be inside the `rolling_upgrade_policy` block?
			//   # required when using rolling upgrade policy
			//  health_probe_id = "${azurerm_lb_probe.test.id}"
			"upgrade_policy": computeSvc.VirtualMachineScaleSetUpgradePolicySchema(),

			"zero_balance": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			// Computed
			"unique_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceArmLinuxVirtualMachineScaleSetCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	resourceGroup := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)

	if features.ShouldResourcesBeImported() {
		resp, err := client.Get(ctx, resourceGroup, name)
		if err != nil {
			if !utils.ResponseWasNotFound(resp.Response) {
				return fmt.Errorf("Error checking for existing Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
			}
		}

		if !utils.ResponseWasNotFound(resp.Response) {
			return tf.ImportAsExistsError("azurerm_linux_virtual_machine_scale_set", *resp.ID)
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	t := d.Get("tags").(map[string]interface{})

	additionalCapabilitiesRaw := d.Get("additional_capabilities").([]interface{})
	additionalCapabilities := computeSvc.ExpandVirtualMachineScaleSetAdditionalCapabilities(additionalCapabilitiesRaw)

	osDiskRaw := d.Get("os_disk").([]interface{})
	osDisk := computeSvc.ExpandVirtualMachineScaleSetOSDisk(osDiskRaw, compute.Linux)

	sshKeysRaw := d.Get("admin_ssh_key").(*schema.Set).List()
	sshKeys := computeSvc.ExpandSSHKeys(sshKeysRaw)

	sourceImageReferenceRaw := d.Get("source_image_reference").([]interface{})
	sourceImageReference := computeSvc.ExpandVirtualMachineScaleSetSourceImageReference(sourceImageReferenceRaw)
	if sourceImageReference == nil {
		sourceImageId := d.Get("source_image_id").(string)
		if sourceImageId == "" {
			return fmt.Errorf("Either a `source_image_id` or a `source_image_reference` block must be specified!")
		}

		sourceImageReference = &compute.ImageReference{
			ID: utils.String(sourceImageId),
		}
	}

	upgradePolicyRaw := d.Get("upgrade_policy").([]interface{})
	upgradePolicy, err := computeSvc.ExpandVirtualMachineScaleSetUpgradePolicy(upgradePolicyRaw)
	if err != nil {
		return fmt.Errorf("Error expanding `upgrade_policy`: %+v", err)
	}

	virtualMachineProfile := compute.VirtualMachineScaleSetVMProfile{
		Priority: compute.VirtualMachinePriorityTypes(d.Get("priority").(string)),
		OsProfile: &compute.VirtualMachineScaleSetOSProfile{
			AdminPassword:      utils.String(d.Get("admin_password").(string)),
			AdminUsername:      utils.String(d.Get("admin_username").(string)),
			ComputerNamePrefix: utils.String(d.Get("computer_name_prefix").(string)),
			LinuxConfiguration: &compute.LinuxConfiguration{
				DisablePasswordAuthentication: utils.Bool(d.Get("disable_password_authentication").(bool)),
				ProvisionVMAgent:              utils.Bool(d.Get("provision_vm_agent").(bool)),
				SSH: &compute.SSHConfiguration{
					PublicKeys: sshKeys,
				},
			},
			// TODO: customData & secrets
		},
		// DiagnosticsProfile:
		NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
			HealthProbe: &compute.APIEntityReference{
				// TODO: required if `Rolling` is used
				ID: nil,
			},
			NetworkInterfaceConfigurations: nil,
		},
		StorageProfile: &compute.VirtualMachineScaleSetStorageProfile{
			ImageReference: sourceImageReference,
			OsDisk:         osDisk,
			// TODO: Data Disks
			//DataDisks: &[]compute.VirtualMachineScaleSetDataDisk{
			//	{
			//		Name:                    nil,
			//		Lun:                     nil,
			//		Caching:                 "",
			//		WriteAcceleratorEnabled: nil,
			//		CreateOption:            "",
			//		DiskSizeGB:              nil,
			//		ManagedDisk:             nil,
			//	},
			//},
		},
	}
	if evictionPolicyRaw, ok := d.GetOk("eviction_policy"); ok {
		if virtualMachineProfile.Priority != compute.Low {
			return fmt.Errorf("An `eviction_policy` can only be specified when `priority` is set to `low`")
		}
		virtualMachineProfile.EvictionPolicy = compute.VirtualMachineEvictionPolicyTypes(evictionPolicyRaw.(string))
	}

	props := compute.VirtualMachineScaleSet{
		// TODO: Identity & Zones
		Location: utils.String(location),
		Tags:     tags.Expand(t),
		VirtualMachineScaleSetProperties: &compute.VirtualMachineScaleSetProperties{
			AdditionalCapabilities:                 additionalCapabilities,
			DoNotRunExtensionsOnOverprovisionedVMs: utils.Bool(d.Get("do_not_run_extensions_on_overprovisioned_machines").(bool)),
			Overprovision:                          utils.Bool(d.Get("overprovision").(bool)),
			PlatformFaultDomainCount:               utils.Int32(int32(d.Get("platform_fault_domain_count").(int))), // TODO: does this want to be conditionally set
			SinglePlacementGroup:                   utils.Bool(d.Get("single_placement_group").(bool)),
			VirtualMachineProfile:                  &virtualMachineProfile,
			UpgradePolicy:                          upgradePolicy,
			ZoneBalance:                            utils.Bool(d.Get("zero_balance").(bool)),
		},
	}

	if v, ok := d.GetOk("proximity_placement_group_id"); ok {
		props.VirtualMachineScaleSetProperties.ProximityPlacementGroup = &compute.SubResource{
			ID: utils.String(v.(string)),
		}
	}

	log.Printf("[DEBUG] Creating Linux Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
	future, err := client.CreateOrUpdate(ctx, resourceGroup, name, props)
	if err != nil {
		return fmt.Errorf("Error creating Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	log.Printf("[DEBUG] Waiting for Linux Virtual Machine Scale Set %q (Resource Group %q) to be created..", name, resourceGroup)
	if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for creation of Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}
	log.Printf("[DEBUG] Virtual Machine Scale Set %q (Resource Group %q) was created", name, resourceGroup)

	log.Printf("[DEBUG] Retrieving Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if resp.ID == nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): ID was nil", name, resourceGroup)
	}
	d.SetId(*resp.ID)

	// this shouldn't need to go into Update, but let's see
	return resourceArmLinuxVirtualMachineScaleSetRead(d, meta)
}

func resourceArmLinuxVirtualMachineScaleSetUpdate(d *schema.ResourceData, meta interface{}) error {
	// TODO: implement me
	return resourceArmLinuxVirtualMachineScaleSetRead(d, meta)
}

func resourceArmLinuxVirtualMachineScaleSetRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	id, err := computeSvc.ParseVirtualMachineScaleSetResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Name
	resourceGroup := id.Base.ResourceGroup

	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] Linux Virtual Machine Scale Set %q was not found in Resource Group %q - removing from state!", name, resourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	d.Set("name", name)
	d.Set("resource_group_name", resourceGroup)
	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(location))
	}

	if resp.VirtualMachineScaleSetProperties == nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): `properties` was nil", name, resourceGroup)
	}
	props := *resp.VirtualMachineScaleSetProperties

	// TODO: if this isn't a Linux Scale Set we should error here

	if err := d.Set("additional_capabilities", computeSvc.FlattenVirtualMachineScaleSetAdditionalCapabilities(props.AdditionalCapabilities)); err != nil {
		return fmt.Errorf("Error setting `additional_capabilities`: %+v", props.AdditionalCapabilities)
	}
	d.Set("do_not_run_extensions_on_overprovisioned_machines", props.DoNotRunExtensionsOnOverprovisionedVMs)
	d.Set("overprovision", props.Overprovision)
	if props.PlatformFaultDomainCount != nil {
		d.Set("platform_fault_domain_count", int(*props.PlatformFaultDomainCount))
	}
	if group := props.ProximityPlacementGroup; group != nil {
		d.Set("proximity_placement_group_id", group.ID)
	}
	d.Set("single_placement_group", props.SinglePlacementGroup)
	d.Set("unique_id", props.UniqueID)
	if err := d.Set("upgrade_policy", computeSvc.FlattenVirtualMachineScaleSetUpgradePolicy(props.UpgradePolicy)); err != nil {
		return fmt.Errorf("Error setting `upgrade_policy`: %+v", props.AdditionalCapabilities)
	}
	d.Set("zone_balance", props.ZoneBalance)

	if profile := props.VirtualMachineProfile; profile != nil {
		if storageProfile := profile.StorageProfile; storageProfile != nil {
			if d.Set("os_disk", computeSvc.FlattenVirtualMachineScaleSetOSDisk(storageProfile.OsDisk)); err != nil {
				return fmt.Errorf("Error setting `os_disk`: %+v", err)
			}

			if d.Set("source_image_reference", computeSvc.FlattenVirtualMachineScaleSetSourceImageReference(storageProfile.ImageReference)); err != nil {
				return fmt.Errorf("Error setting `source_image_reference`: %+v", err)
			}

			var storageImageId string
			if storageProfile.ImageReference != nil && storageProfile.ImageReference.ID != nil {
				storageImageId = *storageProfile.ImageReference.ID
			}
			d.Set("source_image_id", storageImageId)
		}

		if osProfile := profile.OsProfile; osProfile != nil {
			// admin_password isn't returned, but it's a top level field so we can ignore it without consequence
			d.Set("admin_username", osProfile.AdminUsername)
			d.Set("computer_name_prefix", osProfile.ComputerNamePrefix)

			if linux := osProfile.LinuxConfiguration; linux != nil {
				d.Set("disable_password_authentication", linux.DisablePasswordAuthentication)
				d.Set("provision_vm_agent", linux.ProvisionVMAgent)

				flattenedSshKeys, err := computeSvc.FlattenSSHKeys(linux.SSH)
				if err != nil {
					return fmt.Errorf("Error flattening `admin_ssh_key`: %+v", err)
				}
				if err := d.Set("admin_ssh_key", *flattenedSshKeys); err != nil {
					return fmt.Errorf("Error setting `admin_ssh_key`: %+v", err)
				}
			}
		}
	}

	return nil
}

func resourceArmLinuxVirtualMachineScaleSetDelete(d *schema.ResourceData, meta interface{}) error {
	// TODO: implement me
	return nil
}
