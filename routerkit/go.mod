module github.com/eduardpaul/routerctl/routerkit

go 1.24.0

require (
	github.com/eduardpaul/routerctl/asusrouter v0.1.0
	github.com/eduardpaul/routerctl/netkit v0.1.0
)

// v0.1.0's tag was rewritten to remove captured device data, so its
// source no longer matches the checksum database. Use v0.1.1.
retract v0.1.0
