//go:build windows

package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WMI in pure Go, no third-party dependency: the COM objects are called through
// their vtables with syscall. Indices (Microsoft docs, wbemcli.h):
//
//	IUnknown:               0 QueryInterface, 1 AddRef, 2 Release
//	IWbemLocator:           3 ConnectServer
//	IWbemServices:          20 ExecQuery
//	IEnumWbemClassObject:   4 Next
//	IWbemClassObject:       4 Get
const (
	vtRelease       = 2
	vtConnectServer = 3
	vtExecQuery     = 20
	vtEnumNext      = 4
	vtObjGet        = 4

	wbemFlagReturnImmediately = 0x10
	wbemFlagForwardOnly       = 0x20

	rpcCAuthnWinNT         = 10
	rpcCAuthzNone          = 0
	rpcCAuthnLevelCall     = 3
	rpcCImpLevelImpersonat = 3
	eoacNone               = 0

	coinitMultithreaded = 0
	clsctxInprocServer  = 1
	vtBSTR              = 8

	sOK    = 0
	sFalse = 1
	// RPC_E_CHANGED_MODE: COM already initialized in another mode on this thread.
	rpcEChangedMode = 0x80010106

	// wmiTimeout bounds the whole query; on expiry the MAC falls through to
	// PLACA and then to the interface list.
	wmiTimeout     = 3 * time.Second
	wmiNextTimeout = 2500 // ms, for IEnumWbemClassObject::Next
)

var (
	clsidWbemLocator = windows.GUID{Data1: 0x4590F811, Data2: 0x1D3A, Data3: 0x11D0, Data4: [8]byte{0x89, 0x1F, 0x00, 0xAA, 0x00, 0x4B, 0x2E, 0x24}}
	iidWbemLocator   = windows.GUID{Data1: 0xDC12A687, Data2: 0x737F, Data3: 0x11CF, Data4: [8]byte{0x88, 0x4D, 0x00, 0xAA, 0x00, 0x4B, 0x2E, 0x24}}

	modOle32   = windows.NewLazySystemDLL("ole32.dll")
	modOleAut  = windows.NewLazySystemDLL("oleaut32.dll")
	pCoInit    = modOle32.NewProc("CoInitializeEx")
	pCoUninit  = modOle32.NewProc("CoUninitialize")
	pCoCreate  = modOle32.NewProc("CoCreateInstance")
	pCoBlanket = modOle32.NewProc("CoSetProxyBlanket")
	pSysAlloc  = modOleAut.NewProc("SysAllocString")
	pSysFree   = modOleAut.NewProc("SysFreeString")
	pVarClear  = modOleAut.NewProc("VariantClear")
)

// wmiMACs returns the MACAddress of every physical adapter, in the order WMI
// answers (the same query and order as the AD logon script). The COM work runs
// in a goroutine locked to one OS thread; on timeout that goroutine is left to
// finish on its own and the caller moves on.
func wmiMACs() []string {
	wmiOnce.Do(func() { wmiCached = wmiMACsUncached() })
	return wmiCached
}

// wmiOnce makes the query run once per process (the answer does not change while
// the exe runs, and the tests call the collector many times).
var (
	wmiOnce   sync.Once
	wmiCached []string
)

func wmiMACsUncached() []string {
	ch := make(chan []string, 1)
	go func() { ch <- queryWMIMACs() }()
	select {
	case v := <-ch:
		return v
	case <-time.After(wmiTimeout):
		return nil
	}
}

// vcall calls method idx of the COM object obj.
func vcall(obj unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		vcall(obj, vtRelease)
	}
}

func setBlanket(obj unsafe.Pointer) {
	_, _, _ = pCoBlanket.Call(uintptr(obj), rpcCAuthnWinNT, rpcCAuthzNone, 0,
		rpcCAuthnLevelCall, rpcCImpLevelImpersonat, 0, eoacNone)
}

func bstr(s string) unsafe.Pointer {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	r, _, _ := pSysAlloc.Call(uintptr(unsafe.Pointer(p)))
	return *(*unsafe.Pointer)(unsafe.Pointer(&r))
}

func freeBSTR(p unsafe.Pointer) {
	if p != nil {
		_, _, _ = pSysFree.Call(uintptr(p))
	}
}

func queryWMIMACs() (out []string) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := pCoInit.Call(0, coinitMultithreaded)
	switch uint32(hr) {
	case sOK, sFalse:
		defer func() { _, _, _ = pCoUninit.Call() }()
	case rpcEChangedMode:
		// already initialized in another mode: use it, do not uninitialize
	default:
		return nil
	}

	loc := heapVar[unsafe.Pointer]()
	hr, _, _ = pCoCreate.Call(uintptr(unsafe.Pointer(&clsidWbemLocator)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidWbemLocator)), uintptr(unsafe.Pointer(loc)))
	if int32(hr) < 0 || *loc == nil {
		return nil
	}
	defer release(*loc)

	ns := bstr(`ROOT\CIMV2`)
	defer freeBSTR(ns)
	svc := heapVar[unsafe.Pointer]()
	// ConnectServer(network, user, password, locale, securityFlags, authority, ctx, &services)
	if int32(vcall(*loc, vtConnectServer, uintptr(ns), 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(svc)))) < 0 || *svc == nil {
		return nil
	}
	defer release(*svc)
	setBlanket(*svc)

	wql, q := bstr("WQL"), bstr("SELECT MACAddress FROM Win32_NetworkAdapter WHERE PhysicalAdapter=TRUE")
	defer freeBSTR(wql)
	defer freeBSTR(q)
	enum := heapVar[unsafe.Pointer]()
	// ExecQuery(language, query, flags, ctx, &enum)
	if int32(vcall(*svc, vtExecQuery, uintptr(wql), uintptr(q),
		wbemFlagForwardOnly|wbemFlagReturnImmediately, 0, uintptr(unsafe.Pointer(enum)))) < 0 || *enum == nil {
		return nil
	}
	defer release(*enum)
	setBlanket(*enum)

	name, err := windows.UTF16PtrFromString("MACAddress")
	if err != nil {
		return nil
	}
	for {
		obj := heapVar[unsafe.Pointer]()
		got := heapVar[uint32]()
		// Next(timeoutMs, count, &objects, &returned)
		r := int32(vcall(*enum, vtEnumNext, wmiNextTimeout, 1, uintptr(unsafe.Pointer(obj)), uintptr(unsafe.Pointer(got))))
		if r < 0 || *got == 0 || *obj == nil {
			return out
		}
		v := heapVar[[24]byte]() // VARIANT: 24 bytes on 64-bit (vt + union of two pointers)
		// Get(name, flags, &variant, &type, &flavor)
		if int32(vcall(*obj, vtObjGet, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(v)), 0, 0)) >= 0 {
			mac := ""
			if *(*uint16)(unsafe.Pointer(v)) == vtBSTR {
				if p := *(*unsafe.Pointer)(unsafe.Add(unsafe.Pointer(v), 8)); p != nil {
					mac = windows.UTF16PtrToString((*uint16)(p))
				}
			}
			_, _, _ = pVarClear.Call(uintptr(unsafe.Pointer(v)))
			out = append(out, mac)
		} else {
			out = append(out, "")
		}
		release(*obj)
	}
}

// sink forces heapVar's allocations onto the heap. Go stacks move when they
// grow, and COM writes through these addresses as plain uintptr values; a heap
// object never moves, a stack variable might.
var sink unsafe.Pointer

// heapVar allocates an out-parameter for a COM call on the heap.
func heapVar[T any]() *T {
	p := new(T)
	atomic.StorePointer(&sink, unsafe.Pointer(p))
	return p
}
