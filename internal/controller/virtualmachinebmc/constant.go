package virtualmachinebmc

const (
	virtBMCContainerName       = "virtbmc"
	VirtBMCImageName           = "anish60/virtbmc"
	ipmiPort                   = 10623
	redfishPort                = 10080
	IPMISvcPort                = 623
	RedfishSvcPort             = 80
	ipmiPortName               = "ipmi"
	redfishPortName            = "redfish"
	VirtualMachineBMCNameLabel = "kubevirt.io/virtualmachinebmc-name"
	VMNameLabel                = "kubevirt.io/vm-name"
	VirtualMachineBMCNamespace = "kubevirtbmc-system"
)
