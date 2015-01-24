package preproc

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"

	"github.com/gophergala/gomp/gensym"
)

type Cond int

type Context struct {
	genSym func() string
}

const (
	COND_LT = iota
	COND_LE
	COND_GT
	COND_GE
)

func parseForInit(stmt *ast.Stmt) (variable *ast.Ident, initExpr *ast.Expr, ok bool) {
	if stmt == nil {
		return
	}
	var assignStmt *ast.AssignStmt
	if assignStmt, ok = (*stmt).(*ast.AssignStmt); !ok {
		return
	}
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return
	}
	if variable, ok = assignStmt.Lhs[0].(*ast.Ident); !ok {
		return
	}
	initExpr = &assignStmt.Rhs[0]
	return
}

func parseForCond(expr *ast.Expr) (variable *ast.Ident, op token.Token, bound *ast.Expr, ok bool) {
	if expr == nil {
		return
	}
	binaryExpr, ok := (*expr).(*ast.BinaryExpr)
	if !ok {
		return
	}
	switch binaryExpr.Op {
	case token.LEQ, token.LSS, token.GTR, token.GEQ:
		op = binaryExpr.Op
	default:
		return
	}
	if variable, ok = binaryExpr.X.(*ast.Ident); !ok {
		return
	}
	bound = &binaryExpr.Y
	return
}

func parseForPost(stmt *ast.Stmt) (variable *ast.Ident, op token.Token, ok bool) {
	if stmt == nil {
		return
	}

	if incDecStmt, isIncDec := (*stmt).(*ast.IncDecStmt); isIncDec {
		variable, ok = incDecStmt.X.(*ast.Ident)
		op = incDecStmt.Tok
		return
	}
	return
}

func visitFor(stmt *ast.ForStmt, context *Context) *ast.BlockStmt {
	initVar, initExpr, initOk := parseForInit(&stmt.Init)
	condVar, _, condExpr, condOk := parseForCond(&stmt.Cond)
	postVar, postOp, postOk := parseForPost(&stmt.Post)

	if !initOk || !condOk || !postOk {
		return nil
	}
	if initVar.Name != condVar.Name || initVar.Name != postVar.Name {
		return nil
	}

	block := new(ast.BlockStmt)

	boundsDecl := ast.AssignStmt{}
	{
		initVarSym := ast.Ident{Name: context.genSym()}
		condVarSym := ast.Ident{Name: context.genSym()}
		incVarSym := ast.Ident{Name: context.genSym()}

		incVarConst := ast.BasicLit{Kind: token.INT}
		switch postOp {
		case token.INC:
			incVarConst.Value = "1"
		case token.DEC:
			incVarConst.Value = "-1"
		}

		boundsDecl.Lhs = []ast.Expr{&initVarSym, &condVarSym, &incVarSym}
		boundsDecl.Tok = token.DEFINE
		boundsDecl.Rhs = []ast.Expr{*initExpr, *condExpr, &incVarConst}
		*initExpr, *condExpr = ast.Expr(&initVarSym), ast.Expr(&condVarSym)
		stmt.Post = &ast.AssignStmt{
			Lhs: []ast.Expr{initVar},
			Tok: token.ADD_ASSIGN,
			Rhs: []ast.Expr{&incVarSym},
		}
	}
	block.List = []ast.Stmt{&boundsDecl, ast.Stmt(stmt)}
	return block
}

func visitExpr(e *ast.Expr, context *Context) {
	if e == nil {
		return
	}
	switch t := (*e).(type) {
	case *ast.FuncLit:
		if t.Body == nil {
			return
		}
		for _, s := range t.Body.List {
			visitStmt(&s, context)
		}
	}
}

func visitStmt(stmt *ast.Stmt, context *Context) {
	if stmt == nil {
		return
	}
	switch t := (*stmt).(type) {
	case *ast.AssignStmt:
		for _, e := range t.Rhs {
			visitExpr(&e, context)
		}
	case *ast.ForStmt:
		if block := visitFor(t, context); block != nil {
			*stmt = block
		}
	case *ast.BlockStmt:
		visitBlock(t, context)
	case *ast.IfStmt:
		visitBlock(t.Body, context)
	case *ast.SwitchStmt:
		visitBlock(t.Body, context)
	case *ast.TypeSwitchStmt:
		visitBlock(t.Body, context)
	case *ast.CaseClause:
		for i, _ := range t.Body {
			visitStmt(&t.Body[i], context)
		}
	}
}

func visitBlock(stmt *ast.BlockStmt, context *Context) {
	if stmt != nil {
		for i, _ := range stmt.List {
			visitStmt(&stmt.List[i], context)
		}
	}
}

func visitFunction(f *ast.FuncDecl, context *Context) {
	if f.Body != nil {
		visitBlock(f.Body, context)
	}
}

// Run preprocessor on a source. filename is used for error reporting.
// This function is currently not implemented.
func PreprocFile(source, filename string) (result string, err error) {
	context := Context{gensym.MkGen(source)}

	file, err := parser.ParseFile(token.NewFileSet(), filename, source,
		parser.ParseComments|parser.AllErrors)
	if err != nil {
		return
	}
	for _, decl := range file.Decls {
		if fun, ok := decl.(*ast.FuncDecl); ok {
			visitFunction(fun, &context)
		}
	}

	var buf bytes.Buffer
	printer.Fprint(&buf, token.NewFileSet(), file)
	result = buf.String()
	return
}
