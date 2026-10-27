//go:build windows

package launcher

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AllowFirewall does the work of AllowFirewallFlag, in the elevated copy.
func AllowFirewall(exe string) error {
	var err error
	for i, line := range firewallCommands(exe) {
		cmd := exec.Command("netsh")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line, HideWindow: true, CreationFlags: createNoWindow}
		if e := cmd.Run(); i > 0 { // deleting finds nothing on a clean install: fine
			err = e
		}
	}
	return err
}

// ErrCancelled: the person said No in Windows' "make changes?" box.
var ErrCancelled = errors.New("cancelled")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size        uint32
	mask        uint32
	hwnd        windows.Handle
	verb        *uint16
	file        *uint16
	parameters  *uint16
	directory   *uint16
	show        int32
	instApp     windows.Handle
	idList      uintptr
	class       *uint16
	keyClass    windows.Handle
	hotKey      uint32
	iconMonitor windows.Handle
	process     windows.Handle
}

var pShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// AllowFirewallElevated starts exe with AllowFirewallFlag as administrator
// (Windows shows its confirmation box) and waits for it.
func AllowFirewallElevated(exe string, owner windows.Handle) error {
	info := shellExecuteInfo{
		mask:       0x00000040, // SEE_MASK_NOCLOSEPROCESS: hand back the process to wait for
		hwnd:       owner,
		verb:       windows.StringToUTF16Ptr("runas"),
		file:       windows.StringToUTF16Ptr(exe),
		parameters: windows.StringToUTF16Ptr(AllowFirewallFlag),
		directory:  windows.StringToUTF16Ptr(filepath.Dir(exe)),
		show:       0, // SW_HIDE
	}
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, err := pShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		return err
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, 60_000); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return errors.New("netsh couldn't add the rule")
	}
	return nil
}

// OpenFirewallSettings opens the firewall's "Allowed apps" page.
func OpenFirewallSettings() error {
	return Outside("control.exe", "/name", "Microsoft.WindowsFirewall", "/page", "pageConfigureApps").Start()
}
