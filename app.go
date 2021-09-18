package colima

import (
	"fmt"
	"github.com/abiosoft/colima/config"
	"github.com/abiosoft/colima/runtime/container"
	"github.com/abiosoft/colima/runtime/container/docker"
	"github.com/abiosoft/colima/runtime/host"
	"github.com/abiosoft/colima/runtime/vm"
	"log"
)

type App interface {
	Start() error
	Stop() error
	Delete() error
}

var _ App = (*colimaApp)(nil)

func New(c config.Config) (App, error) {
	vmConfig := vm.Config{
		CPU:     c.VM.CPU,
		Disk:    c.VM.Disk,
		Memory:  c.VM.Memory,
		SSHPort: config.SSHPort(),
		Changed: false,
	}

	guest := vm.New(host.New(), vmConfig)
	if err := host.IsInstalled(guest); err != nil {
		return nil, fmt.Errorf("dependency check failed for VM: %w", err)
	}

	dockerRuntime := docker.New(guest.Host(), guest)
	if err := host.IsInstalled(dockerRuntime); err != nil {
		return nil, fmt.Errorf("dependency check failed for docker: %w", err)
	}

	return &colimaApp{
		guest:      guest,
		containers: []container.Runtime{dockerRuntime},
	}, nil
}

type colimaApp struct {
	guest      vm.Runtime
	containers []container.Runtime
}

func (c colimaApp) Start() error {
	// the order for start is:
	//   vm start -> container provision -> container start

	// start vm
	if err := c.guest.Start(); err != nil {
		return fmt.Errorf("error starting vm: %w", err)
	}

	// provision container runtimes
	for _, cont := range c.containers {
		if err := cont.Provision(); err != nil {
			return fmt.Errorf("error provisioning %s: %w", cont.Name(), err)
		}
	}

	// start container runtimes
	for _, cont := range c.containers {
		if err := cont.Start(); err != nil {
			return fmt.Errorf("error starting %s: %w", cont.Name(), err)
		}
	}

	return nil
}

func (c colimaApp) Stop() error {
	// the order for stop is:
	//   container stop -> vm stop

	// stop containers
	for _, cont := range c.containers {
		if err := cont.Stop(); err != nil {
			// failure to stop a container runtime is not fatal
			// it is only meant for graceful shutdown.
			// the VM will shutdown anyways.
			log.Println(fmt.Errorf("error stopping %s: %w", cont.Name(), err))
		}
	}

	// stop vm
	if err := c.guest.Stop(); err != nil {
		return fmt.Errorf("error stopping vm: %w", err)
	}

	return nil
}

func (c colimaApp) Delete() error {
	// the order for teardown is:
	//   container teardown -> vm teardown

	// vm teardown would've sufficed but container provision
	// may have created files on the host.
	// it is essential to teardown containers as well.

	for _, cont := range c.containers {
		if err := cont.Teardown(); err != nil {
			// failure here is not fatal
			log.Println(fmt.Errorf("error during teardown of %s: %w", cont.Name(), err))
		}
	}

	if err := c.guest.Teardown(); err != nil {
		return fmt.Errorf("error during teardown of vm: %w", err)
	}

	return nil
}
