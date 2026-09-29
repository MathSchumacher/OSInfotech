package main

import (
	_ "embed"
	"encoding/base64"
	"html/template"

	"fyne.io/fyne/v2"
)

//go:embed assets/icons/dbs.png
var appIconBytes []byte

var appIcon = fyne.NewStaticResource("dbs.png", appIconBytes)

func appIconDataURI() template.URL {
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(appIconBytes))
}
