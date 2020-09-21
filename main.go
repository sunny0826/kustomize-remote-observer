package main

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sunny0826/kustomize-remote-observer/controllers"
	"github.com/zserge/lorca"
	_ "github.com/zserge/lorca"
	"html/template"
	"io"
	"log"
	_ "log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
)

func main() {
	args := []string{}
	if runtime.GOOS == "linux" {
		args = append(args, "--class=Lorca")
	}
	ui, err := lorca.New("", "", 480, 700, args...)
	if err != nil {
		log.Fatal(err)
	}
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	ep, err := os.Executable()
	if err != nil {
		log.Fatalln("os.Executable:", err)
	}
	err = os.Chdir(filepath.Join(filepath.Dir(ep), "..", "Resources"))
	if err != nil {
		log.Fatalln("os.Chdir:", err)
	}

	// Static
	e.Static("/assets", "assets")
	e.File("/favicon.ico", "assets/images/favicon.ico")

	renderer := &TemplateRenderer{
		templates: template.Must(template.ParseGlob("views/*.html")),
	}
	e.Renderer = renderer

	// Routes
	e.POST("/kust", controllers.HandlerKust)
	e.POST("/gene", controllers.GenerateKust)
	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", "")
	})
	// Start server
	go e.Start(":1323")

	ui.Load(fmt.Sprintf("http://%s", "localhost:1323"))
	defer ui.Close()
	//e.Logger.Fatal(e.Start(":1323"))

	// Wait until the interrupt signal arrives or browser window is closed
	sigc := make(chan os.Signal)
	signal.Notify(sigc, os.Interrupt)
	select {
	case <-sigc:
	case <-ui.Done():
	}

}

// TemplateRenderer is a custom html/template renderer for Echo framework
type TemplateRenderer struct {
	templates *template.Template
}

// Render renders a template document
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {

	// Add global methods if data is a map
	if viewContext, isMap := data.(map[string]interface{}); isMap {
		viewContext["reverse"] = c.Echo().Reverse
	}

	return t.templates.ExecuteTemplate(w, name, data)
}
