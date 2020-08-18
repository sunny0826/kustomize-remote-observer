package controllers

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"net/http"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
)

type kustType struct {
	User      string `json:"username" form:"username" query:"username"`
	Pass      string `json:"password" form:"password" query:"password"`
	Protocols string `json:"protocols" form:"protocols" query:"protocols"`
	GitPath   string `json:"git_path" form:"git_path" query:"git_path"`
}

func HandlerKust(c echo.Context) error {
	log.Info("start")
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
	log.Info("end")
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
