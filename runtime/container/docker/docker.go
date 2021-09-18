package docker

import (
	"github.com/abiosoft/colima/config"
	"github.com/abiosoft/colima/runner"
	"github.com/abiosoft/colima/runtime"
	"github.com/abiosoft/colima/runtime/container"
	"os"
	"path/filepath"
)

var _ container.Runtime = (*dockerRuntime)(nil)

func socketSymlink() string {
	return filepath.Join(config.Dir(), "docker.sock")
}

type dockerRuntime struct {
	host  runtime.HostActions
	guest runtime.GuestActions
	runner.Instance
	launchd launchAgent
}

// New creates a new docker runtime.
func New(host runtime.HostActions, guest runtime.GuestActions) container.Runtime {
	launchdPkg := "com.abiosoft." + config.AppName()

	return &dockerRuntime{
		host:     host,
		guest:    guest,
		Instance: runner.New("docker"),
		launchd:  launchAgent(launchdPkg),
	}
}

func (d dockerRuntime) Name() string {
	return "docker"
}

func (d dockerRuntime) isInstalled() bool {
	err := d.guest.Run("command", "-v", "docker")
	return err == nil
}

func (d dockerRuntime) isUserPermissionFixed() bool {
	err := d.guest.Run("sh", "-c", `getent group docker | grep "\b${USER}\b"`)
	return err == nil
}

func (d dockerRuntime) Provision() error {
	r := d.Init()
	r.Stage("provisioning")

	// check installation
	if !d.isInstalled() {
		r.Stage("setting up socket")
		r.Add(d.setupSocketSymlink)

		r.Stage("provisioning in VM")
		r.Add(d.setupInVM)
	}

	// check user permission
	if !d.isUserPermissionFixed() {
		r.Add(d.fixUserPermission)

		r.Stage("restarting VM to complete setup")
		r.Add(d.guest.Stop)
		r.Add(d.guest.Start)
	}

	// socket file/launchd
	r.Add(createSocketForwardingScript)
	r.Add(func() error { return createLaunchdScript(d.launchd) })

	return r.Run()
}

func (d dockerRuntime) Start() error {
	r := d.Init()
	r.Stage("starting")

	r.Add(func() error {
		return d.guest.Run("sudo", "service", "docker", "start")
	})
	r.Add(func() error {
		return d.host.Run("launchctl", "load", d.launchd.File())
	})

	return r.Run()
}

func (d dockerRuntime) Stop() error {
	r := d.Init()
	r.Stage("stopping")

	r.Add(func() error {
		return d.guest.Run("service", "docker", "status")
	})
	r.Add(func() error {
		return d.host.Run("launchctl", "unload", d.launchd.File())
	})

	return r.Run()
}

func (d dockerRuntime) Teardown() error {
	r := d.Init()
	r.Stage("teardown")

	if stat, err := os.Stat(d.launchd.File()); err == nil && !stat.IsDir() {
		r.Add(func() error {
			return d.host.Run("launchctl", "unload", d.launchd.File())
		})
	}

	return r.Run()
}

func (d dockerRuntime) Dependencies() []string {
	return []string{"docker"}
}
