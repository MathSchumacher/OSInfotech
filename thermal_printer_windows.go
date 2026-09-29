//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

var (
	winspool              = windows.NewLazySystemDLL("winspool.drv")
	procClosePrinter      = winspool.NewProc("ClosePrinter")
	procEnumPrinters      = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
	procOpenPrinter       = winspool.NewProc("OpenPrinterW")
	procStartDocPrinter   = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter     = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter  = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter    = winspool.NewProc("EndPagePrinter")
	procWritePrinter      = winspool.NewProc("WritePrinter")
)

type printerInfo4 struct {
	PrinterName *uint16
	ServerName  *uint16
	Attributes  uint32
}

type docInfo1 struct {
	DocumentName *uint16
	OutputFile   *uint16
	Datatype     *uint16
}

func rawThermalPrintingAvailable() bool {
	return true
}

func thermalPrinterNames() ([]string, string, error) {
	var needed, returned uint32
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	_, _, _ = procEnumPrinters.Call(flags, 0, 4, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, "", nil
	}

	buffer := make([]byte, needed)
	result, _, callErr := procEnumPrinters.Call(
		flags,
		0,
		4,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if result == 0 {
		return nil, "", fmt.Errorf("listar impressoras: %w", callErr)
	}

	infos := unsafe.Slice((*printerInfo4)(unsafe.Pointer(&buffer[0])), int(returned))
	printers := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.PrinterName != nil {
			printers = append(printers, windows.UTF16PtrToString(info.PrinterName))
		}
	}
	return printers, defaultWindowsPrinter(), nil
}

func defaultWindowsPrinter() string {
	var needed uint32
	_, _, _ = procGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return ""
	}
	buffer := make([]uint16, needed)
	result, _, _ := procGetDefaultPrinter.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&needed)))
	if result == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer)
}

func printThermalRaw(printer string, order ServiceOrder, settings ThermalPrintSettings) error {
	printerName, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return err
	}
	var handle windows.Handle
	result, _, callErr := procOpenPrinter.Call(uintptr(unsafe.Pointer(printerName)), uintptr(unsafe.Pointer(&handle)), 0)
	if result == 0 {
		return fmt.Errorf("abrir impressora: %w", callErr)
	}
	defer procClosePrinter.Call(uintptr(handle))

	settings = settings.normalized()
	documentName, _ := windows.UTF16PtrFromString(fmt.Sprintf("%s OS %06d", settings.Title, order.ID))
	rawType, _ := windows.UTF16PtrFromString("RAW")
	doc := docInfo1{DocumentName: documentName, Datatype: rawType}
	job, _, callErr := procStartDocPrinter.Call(uintptr(handle), 1, uintptr(unsafe.Pointer(&doc)))
	if job == 0 {
		return fmt.Errorf("iniciar documento: %w", callErr)
	}
	defer procEndDocPrinter.Call(uintptr(handle))

	result, _, callErr = procStartPagePrinter.Call(uintptr(handle))
	if result == 0 {
		return fmt.Errorf("iniciar pagina: %w", callErr)
	}
	defer procEndPagePrinter.Call(uintptr(handle))

	data := buildESCPOSReceiptWithSettings(order, settings)
	var written uint32
	result, _, callErr = procWritePrinter.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if result == 0 || int(written) != len(data) {
		return fmt.Errorf("enviar recibo: %w", callErr)
	}
	return nil
}
