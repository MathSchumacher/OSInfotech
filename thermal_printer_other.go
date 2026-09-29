//go:build !windows

package main

import "errors"

func rawThermalPrintingAvailable() bool {
	return false
}

func thermalPrinterNames() ([]string, string, error) {
	return nil, "", nil
}

func printThermalRaw(string, ServiceOrder, ThermalPrintSettings) error {
	return errors.New("impressao direta termica disponivel apenas no Windows")
}
