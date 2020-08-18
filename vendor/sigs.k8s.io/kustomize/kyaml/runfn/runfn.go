// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package runfn

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/fn/runtime/container"
	"sigs.k8s.io/kustomize/kyaml/fn/runtime/exec"
	"sigs.k8s.io/kustomize/kyaml/fn/runtime/runtimeutil"
	"sigs.k8s.io/kustomize/kyaml/fn/runtime/starlark"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// RunFns runs the set of configuration functions in a local directory against
// the Resources in that directory
type RunFns struct {
	StorageMounts []runtimeutil.StorageMount

	// Path is the path to the directory containing functions
	Path string

	// FunctionPaths Paths allows functions to be specified outside the configuration
	// directory.
	// Functions provided on FunctionPaths are globally scoped.
	// If FunctionPaths length is > 0, then NoFunctionsFromInput defaults to true
	FunctionPaths []string

	// Functions is an explicit list of functions to run against the input.
	// Functions provided on Functions are globally scoped.
	// If Functions length is > 0, then NoFunctionsFromInput defaults to true
	Functions []*yaml.RNode

	// GlobalScope if true, functions read from input will be scoped globally rather
	// than only to Resources under their subdirs.
	GlobalScope bool

	// Input can be set to read the Resources from Input rather than from a directory
	Input io.Reader

	// Network enables network access for functions that declare it
	Network bool

	// NetworkName is the name of the docker network to use for the container
	NetworkName string

	// Output can be set to write the result to Output rather than back to the directory
	Output io.Writer

	// NoFunctionsFromInput if set to true will not read any functions from the input,
	// and only use explicit sources
	NoFunctionsFromInput *bool

	// EnableStarlark will enable functions run as starlark scripts
	EnableStarlark bool

	// EnableExec will enable exec functions
	EnableExec bool

	// DisableContainers will disable functions run as containers
	DisableContainers bool

	// ResultsDir is where to write each functions results
	ResultsDir string

	// resultsCount is used to generate the results filename for each container
	resultsCount uint32

	// functionFilterProvider provides a filter to perform the function.
	// this is a variable so it can be mocked in tests
	functionFilterProvider func(
		filter runtimeutil.FunctionSpec, api *yaml.RNode) (kio.Filter, error)
}

// Execute runs the command
func (r RunFns) Execute() error {
	// make the path absolute so it works on mac
	var err error
	r.Path, err = filepath.Abs(r.Path)
	if err != nil {
		return errors.Wrap(err)
	}

	// default the containerFilterProvider if it hasn't been override.  Split out for testing.
	(&r).init()
	nodes, fltrs, output, err := r.getNodesAndFilters()
	if err != nil {
		return err
	}
	return r.runFunctions(nodes, output, fltrs)
}

func (r RunFns) getNodesAndFilters() (
	*kio.PackageBuffer, []kio.Filter, *kio.LocalPackageReadWriter, error) {
	// Read Resources from Directory or Input
	buff := &kio.PackageBuffer{}
	p := kio.Pipeline{Outputs: []kio.Writer{buff}}
	// save the output dir because we will need it to write back
	// the same one for reading must be used for writing if deleting Resources
	var outputPkg *kio.LocalPackageReadWriter
	if r.Path != "" {
		outputPkg = &kio.LocalPackageReadWriter{PackagePath: r.Path, MatchFilesGlob: kio.MatchAll}
	}

	if r.Input == nil {
		p.Inputs = []kio.Reader{outputPkg}
	} else {
		p.Inputs = []kio.Reader{&kio.ByteReader{Reader: r.Input}}
	}
	if err := p.Execute(); err != nil {
		return nil, nil, outputPkg, err
	}

	fltrs, err := r.getFilters(buff.Nodes)
	if err != nil {
		return nil, nil, outputPkg, err
	}
	return buff, fltrs, outputPkg, nil
}

func (r RunFns) getFilters(nodes []*yaml.RNode) ([]kio.Filter, error) {
	var fltrs []kio.Filter

	// fns from annotations on the input resources
	f, err := r.getFunctionsFromInput(nodes)
	if err != nil {
		return nil, err
	}
	fltrs = append(fltrs, f...)

	// fns from directories specified on the struct
	f, err = r.getFunctionsFromFunctionPaths()
	if err != nil {
		return nil, err
	}
	fltrs = append(fltrs, f...)

	// explicit fns specified on the struct
	f, err = r.getFunctionsFromFunctions()
	if err != nil {
		return nil, err
	}
	fltrs = append(fltrs, f...)

	return fltrs, nil
}

// runFunctions runs the fltrs against the input and writes to either r.Output or output
func (r RunFns) runFunctions(
	input kio.Reader, output kio.Writer, fltrs []kio.Filter) error {
	// use the previously read Resources as input
	var outputs []kio.Writer
	if r.Output == nil {
		// write back to the package
		outputs = append(outputs, output)
	} else {
		// write to the output instead of the directory if r.Output is specified or
		// the output is nil (reading from Input)
		outputs = append(outputs, kio.ByteWriter{Writer: r.Output})
	}
	err := kio.Pipeline{
		Inputs: []kio.Reader{input}, Filters: fltrs, Outputs: outputs}.Execute()
	if err != nil {
		return err
	}

	// check for deferred function errors
	var errs []string
	for i := range fltrs {
		cf, ok := fltrs[i].(runtimeutil.DeferFailureFunction)
		if !ok {
			continue
		}
		if cf.GetExit() != nil {
			errs = append(errs, cf.GetExit().Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "\n---\n"))
	}
	return nil
}

// getFunctionsFromInput scans the input for functions and runs them
func (r RunFns) getFunctionsFromInput(nodes []*yaml.RNode) ([]kio.Filter, error) {
	if *r.NoFunctionsFromInput {
		return nil, nil
	}

	buff := &kio.PackageBuffer{}
	err := kio.Pipeline{
		Inputs:  []kio.Reader{&kio.PackageBuffer{Nodes: nodes}},
		Filters: []kio.Filter{&runtimeutil.IsReconcilerFilter{}},
		Outputs: []kio.Writer{buff},
	}.Execute()
	if err != nil {
		return nil, err
	}
	sortFns(buff)
	return r.getFunctionFilters(false, buff.Nodes...)
}

// getFunctionsFromFunctionPaths returns the set of functions read from r.FunctionPaths
// as a slice of Filters
func (r RunFns) getFunctionsFromFunctionPaths() ([]kio.Filter, error) {
	buff := &kio.PackageBuffer{}
	for i := range r.FunctionPaths {
		err := kio.Pipeline{
			Inputs: []kio.Reader{
				kio.LocalPackageReader{PackagePath: r.FunctionPaths[i]},
			},
			Outputs: []kio.Writer{buff},
		}.Execute()
		if err != nil {
			return nil, err
		}
	}
	return r.getFunctionFilters(true, buff.Nodes...)
}

// getFunctionsFromFunctions returns the set of explicitly provided functions as
// Filters
func (r RunFns) getFunctionsFromFunctions() ([]kio.Filter, error) {
	return r.getFunctionFilters(true, r.Functions...)
}

func (r RunFns) getFunctionFilters(global bool, fns ...*yaml.RNode) (
	[]kio.Filter, error) {
	var fltrs []kio.Filter
	for i := range fns {
		api := fns[i]
		spec := runtimeutil.GetFunctionSpec(api)
		if spec.Container.Network.Required {
			if !r.Network {
				// TODO(eddiezane): Provide error info about which function needs the network
				return fltrs, errors.Errorf("network required but not enabled with --network")
			}
			spec.Network = r.NetworkName
		}
		c, err := r.functionFilterProvider(*spec, api)
		if err != nil {
			return nil, err
		}

		if c == nil {
			continue
		}
		cf, ok := c.(*container.Filter)
		if global && ok {
			cf.Exec.GlobalScope = true
		}
		fltrs = append(fltrs, c)
	}
	return fltrs, nil
}

// sortFns sorts functions so that functions with the longest paths come first
func sortFns(buff *kio.PackageBuffer) {
	// sort the nodes so that we traverse them depth first
	// functions deeper in the file system tree should be run first
	sort.Slice(buff.Nodes, func(i, j int) bool {
		mi, _ := buff.Nodes[i].GetMeta()
		pi := filepath.ToSlash(mi.Annotations[kioutil.PathAnnotation])
		if filepath.Base(path.Dir(pi)) == "functions" {
			// don't count the functions dir, the functions are scoped 1 level above
			pi = filepath.Dir(path.Dir(pi))
		} else {
			pi = filepath.Dir(pi)
		}

		mj, _ := buff.Nodes[j].GetMeta()
		pj := filepath.ToSlash(mj.Annotations[kioutil.PathAnnotation])
		if filepath.Base(path.Dir(pj)) == "functions" {
			// don't count the functions dir, the functions are scoped 1 level above
			pj = filepath.Dir(path.Dir(pj))
		} else {
			pj = filepath.Dir(pj)
		}

		// i is "less" than j (comes earlier) if its depth is greater -- e.g. run
		// i before j if it is deeper in the directory structure
		li := len(strings.Split(pi, "/"))
		if pi == "." {
			// local dir should have 0 path elements instead of 1
			li = 0
		}
		lj := len(strings.Split(pj, "/"))
		if pj == "." {
			// local dir should have 0 path elements instead of 1
			lj = 0
		}
		if li != lj {
			// use greater-than because we want to sort with the longest
			// paths FIRST rather than last
			return li > lj
		}

		// sort by path names if depths are equal
		return pi < pj
	})
}

// init initializes the RunFns with a containerFilterProvider.
func (r *RunFns) init() {
	if r.NoFunctionsFromInput == nil {
		// default no functions from input if any function sources are explicitly provided
		nfn := len(r.FunctionPaths) > 0 || len(r.Functions) > 0
		r.NoFunctionsFromInput = &nfn
	}

	// if no path is specified, default reading from stdin and writing to stdout
	if r.Path == "" {
		if r.Output == nil {
			r.Output = os.Stdout
		}
		if r.Input == nil {
			r.Input = os.Stdin
		}
	}

	// functionFilterProvider set the filter provider
	if r.functionFilterProvider == nil {
		r.functionFilterProvider = r.ffp
	}
}

// ffp provides function filters
func (r *RunFns) ffp(spec runtimeutil.FunctionSpec, api *yaml.RNode) (kio.Filter, error) {
	var resultsFile string
	if r.ResultsDir != "" {
		resultsFile = filepath.Join(r.ResultsDir, fmt.Sprintf(
			"results-%v.yaml", r.resultsCount))
		atomic.AddUint32(&r.resultsCount, 1)
	}
	if !r.DisableContainers && spec.Container.Image != "" {
		// TODO: Add a test for this behavior
		cf := &container.Filter{
			Image:         spec.Container.Image,
			Network:       spec.Network,
			StorageMounts: r.StorageMounts,
		}
		cf.Exec.FunctionConfig = api
		cf.Exec.GlobalScope = r.GlobalScope
		cf.Exec.ResultsFile = resultsFile
		cf.Exec.DeferFailure = spec.DeferFailure
		return cf, nil
	}
	if r.EnableStarlark && (spec.Starlark.Path != "" || spec.Starlark.URL != "") {
		// the script path is relative to the function config file
		m, err := api.GetMeta()
		if err != nil {
			return nil, errors.Wrap(err)
		}

		var p string
		if spec.Starlark.Path != "" {
			p = filepath.ToSlash(path.Clean(m.Annotations[kioutil.PathAnnotation]))
			spec.Starlark.Path = filepath.ToSlash(path.Clean(spec.Starlark.Path))
			if filepath.IsAbs(spec.Starlark.Path) || path.IsAbs(spec.Starlark.Path) {
				return nil, errors.Errorf(
					"absolute function path %s not allowed", spec.Starlark.Path)
			}
			if strings.HasPrefix(spec.Starlark.Path, "..") {
				return nil, errors.Errorf(
					"function path %s not allowed to start with ../", spec.Starlark.Path)
			}
			p = filepath.ToSlash(filepath.Join(r.Path, filepath.Dir(p), spec.Starlark.Path))
		}
		fmt.Println(p)

		sf := &starlark.Filter{Name: spec.Starlark.Name, Path: p, URL: spec.Starlark.URL}

		sf.FunctionConfig = api
		sf.GlobalScope = r.GlobalScope
		sf.ResultsFile = resultsFile
		sf.DeferFailure = spec.DeferFailure
		return sf, nil
	}

	if r.EnableExec && spec.Exec.Path != "" {
		ef := &exec.Filter{Path: spec.Exec.Path}

		ef.FunctionConfig = api
		ef.GlobalScope = r.GlobalScope
		ef.ResultsFile = resultsFile
		ef.DeferFailure = spec.DeferFailure
		return ef, nil
	}

	return nil, nil
}
