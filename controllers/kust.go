package controllers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"net/http"
	"os"
	"os/user"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	"text/template"
)

type kustType struct {
	User      string `json:"username" form:"username" query:"username"`
	Pass      string `json:"password" form:"password" query:"password"`
	Protocols string `json:"protocols" form:"protocols" query:"protocols"`
	GitPath   string `json:"git_path" form:"git_path" query:"git_path"`
}

type generateType struct {
	AppName        string `json:"appname" form:"appname" query:"appname"`
	Namespace      string `json:"namespace" form:"namespace" query:"namespace"`
	Image          string `json:"image" form:"image" query:"image"`
	RunShell       string `json:"runShell" form:"runShell" query:"runShell"`
	Path           string `json:"path" form:"path" query:"path"`
	CpuLimits      string `json:"cpulimits" form:"cpulimits" query:"cpulimits"`
	CpuRequests    string `json:"cpurequests" form:"cpurequests" query:"cpurequests"`
	MemoryLimits   string `json:"memorylimits" form:"memorylimits" query:"memorylimits"`
	MemoryRequests string `json:"memoryrequests" form:"memoryrequests" query:"memoryrequests"`
	Port           string `json:"port" form:"port" query:"port"`
	TargetPort     string `json:"targetPort" form:"targetPort" query:"targetPort"`
	PullSecrets    string `json:"pullSecrets" form:"pullSecrets" query:"pullSecrets"`
}

func HandlerKust(c echo.Context) error {
	log.Info("Build start")
	k := new(kustType)
	// binds the request payload into kustType struct
	if err := c.Bind(k); err != nil {
		return err
	}
	var gitUrl string
	if k.User != "" {
		gitUrl = fmt.Sprintf("git::%s://%s:%s@%s", k.Protocols, k.User, k.Pass, k.GitPath)
	} else {
		gitUrl = fmt.Sprintf("git::%s://%s", k.Protocols, k.GitPath)
	}
	res, err := kBuild(gitUrl)
	log.Info("Build end")
	if err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	return c.Render(http.StatusOK, "yaml.html", string(res[:]))
}

func kBuild(gitUrl string) ([]byte, error) {
	opts := &krusty.Options{}
	fSys := filesys.MakeFsOnDisk()
	k := krusty.MakeKustomizer(fSys, opts)
	m, err := k.Run(gitUrl)
	if err != nil {
		return nil, err
	}
	res, err := m.AsYaml()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func GenerateKust(c echo.Context) error {
	log.Info("GenerateKust start")
	g := new(generateType)
	if err := c.Bind(g); err != nil {
		return err
	}
	err := handlerTemplate(g)
	if err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	log.Info("GenerateKust end")
	return c.JSON(http.StatusOK, "ok")
}

func handlerTemplate(g *generateType) error {
	desktop := getDesktop()
	basePath := fmt.Sprintf("%s/%s/base", desktop, g.AppName)
	overPath := fmt.Sprintf("%s/%s/overlays/uat", desktop, g.AppName)
	os.MkdirAll(basePath, 0755)
	os.MkdirAll(overPath, 0755)

	fileMap := make(map[string]string)
	fileMap["deploy"] = fmt.Sprintf("%s/deployment.yaml", basePath)
	fileMap["baseKust"] = fmt.Sprintf("%s/kustomization.yaml", basePath)
	fileMap["service"] = fmt.Sprintf("%s/service.yaml", basePath)
	fileMap["strategyPatch"] = fmt.Sprintf("%s/strategy_patch.yaml", overPath)
	fileMap["healthcheckPatch"] = fmt.Sprintf("%s/healthcheck_patch.yaml", overPath)
	fileMap["memorylimitPatch"] = fmt.Sprintf("%s/memorylimit_patch.yaml", overPath)
	fileMap["overKust"] = fmt.Sprintf("%s/kustomization.yaml", overPath)

	for key, name := range fileMap {
		file, err := os.Create(name)
		if err != nil {
			return err
		}
		defer file.Close()
		err = createYaml(key, fileMap, g)
		if err != nil {
			return err
		}
	}
	return nil
}

func createYaml(name string, fileMap map[string]string, g *generateType) error {
	var templateName string
	switch name {
	case "deploy":
		templateName = DeployTemplate
	case "baseKust":
		templateName = BaseKustTemplate
	case "service":
		templateName = SvcTemplate
	case "strategyPatch":
		templateName = StrategyTemplate
	case "healthcheckPatch":
		templateName = HealthCheckTemplate
	case "memorylimitPatch":
		templateName = ResourceTemplate
	case "overKust":
		templateName = OverlaysKustTemplate
	}

	tmpl := template.Must(template.New("tmpl").Parse(templateName))
	fi, err := os.OpenFile(fileMap[name], os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	defer fi.Close()

	err = tmpl.Execute(fi, g)
	if err != nil {
		return err
	}
	return nil
}

func getDesktop() string {
	myself, error := user.Current()
	if error != nil {
		panic(error)
	}
	homedir := myself.HomeDir
	desktop := homedir + "/Desktop/"
	return desktop
}
