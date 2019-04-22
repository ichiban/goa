package codegen

import (
	"path"
	"path/filepath"

	"goa.design/goa/v3/codegen"
	"goa.design/goa/v3/expr"
)

// ServerTypeFiles returns the types file for every gRPC service that contain
// constructors to transform:
//
//   * protocol buffer request message types into service payload types
//   * service result types into protocol buffer response message types
func ServerTypeFiles(genpkg string, root *expr.RootExpr) []*codegen.File {
	fw := make([]*codegen.File, len(root.API.GRPC.Services))
	seen := make(map[string]struct{})
	for i, r := range root.API.GRPC.Services {
		fw[i] = serverType(genpkg, r, seen)
	}
	return fw
}

// serverType returns the file containing the constructor functions to
// transform the gRPC request types to the corresponding service payload types
// and service result types to the corresponding gRPC response types.
//
// seen keeps track of the constructor names that have already been generated
// to prevent duplicate code generation.
func serverType(genpkg string, svc *expr.GRPCServiceExpr, seen map[string]struct{}) *codegen.File {
	var (
		initData []*InitData

		sd = GRPCServices.Get(svc.Name())
	)
	{
		collect := func(c *ConvertData) {
			if c.Init != nil {
				initData = append(initData, c.Init)
			}
		}
		for _, a := range svc.GRPCEndpoints {
			ed := sd.Endpoint(a.Name())
			if c := ed.Request.ServerConvert; c != nil {
				collect(c)
			}
			if c := ed.Response.ServerConvert; c != nil {
				collect(c)
			}
			if ed.ServerStream != nil {
				if c := ed.ServerStream.SendConvert; c != nil {
					collect(c)
				}
				if c := ed.ServerStream.RecvConvert; c != nil {
					collect(c)
				}
			}
			for _, e := range ed.Errors {
				if c := e.Response.ServerConvert; c != nil {
					collect(c)
				}
			}
		}
	}

	var (
		fpath    string
		sections []*codegen.SectionTemplate
	)
	{
		svcName := codegen.SnakeCase(sd.Service.VarName)
		fpath = filepath.Join(codegen.Gendir, "grpc", svcName, "server", "types.go")
		sections = []*codegen.SectionTemplate{
			codegen.Header(svc.Name()+" gRPC server types", "server",
				[]*codegen.ImportSpec{
					{Path: "unicode/utf8"},
					{Path: "goa.design/goa", Name: "goa"},
					{Path: path.Join(genpkg, svcName), Name: sd.Service.PkgName},
					{Path: path.Join(genpkg, svcName, "views"), Name: sd.Service.ViewsPkg},
					{Path: path.Join(genpkg, "grpc", svcName, pbPkgName), Name: sd.PkgName},
				}),
		}
		for _, init := range initData {
			sections = append(sections, &codegen.SectionTemplate{
				Name:   "server-type-init",
				Source: typeInitT,
				Data:   init,
			})
		}
		for _, data := range sd.validations {
			sections = append(sections, &codegen.SectionTemplate{
				Name:   "server-validate",
				Source: validateT,
				Data:   data,
			})
		}
		for _, h := range sd.transformHelpers {
			sections = append(sections, &codegen.SectionTemplate{
				Name:   "server-transform-helper",
				Source: transformHelperT,
				Data:   h,
			})
		}
	}
	return &codegen.File{Path: fpath, SectionTemplates: sections}
}

// input: TransformFunctionData
const transformHelperT = `{{ printf "%s builds a value of type %s from a value of type %s." .Name .ResultTypeRef .ParamTypeRef | comment }}
func {{ .Name }}(v {{ .ParamTypeRef }}) {{ .ResultTypeRef }} {
  {{ .Code }}
  return res
}
`
