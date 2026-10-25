// Калькулятор пребывания: учёт дней в стране для нескольких профилей (членов семьи)
// и автомобиля по редактируемым правилам из data/countries.json.
package main

import (
	_ "embed"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// Иконка окна; иконка exe — в rsrc_windows_amd64.syso (собирается из assets/app.rc).
//
//go:embed assets/icon256.png
var iconPNG []byte

func main() {
	dataDir := flag.String("data", defaultDataDir(), "папка с данными (countries.json и profiles/)")
	flag.Parse()

	store := &Store{Dir: *dataDir}
	if err := store.Init(); err != nil {
		log.Fatal(err)
	}
	a := app.NewWithID("local.stayincalc")
	a.SetIcon(fyne.NewStaticResource("icon.png", iconPNG))
	runUI(a, store)
}

// defaultDataDir — папка data рядом с exe (портативный режим);
// при запуске через `go run` — data в текущей папке.
func defaultDataDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if !strings.Contains(dir, "go-build") {
			return filepath.Join(dir, "data")
		}
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "data")
}
