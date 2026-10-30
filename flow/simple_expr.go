package flow

// SimpleEvaluator 内置零依赖表达式求值器。
//
// 只支持确定性、无副作用的布尔表达式，覆盖网关条件与动态办理人的常见场景：
//
//	days > 3 && urgent == 'true'
//	amount >= 1000 || vip
//	status in ['A', 'B']
//	!(dept == 'IT')
//
// 语义约定：
//   - 标识符沿变量环境做点路径取值，缺失即 null（可与 nil 比较）；
//   - && / || / ! 严格要求布尔操作数（不做隐式 truthiness，避免歧义）；
//   - 相等比较允许数值跨整型/浮点；不同类别一律 false；
//   - 语法错/类型错返回 error，引擎按 fail-closed 处理。
//
// 生产环境可替换为 contrib/exprlang（expr-lang）或 go-scripts 的 CEL 引擎。

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type SimpleEvaluator struct{}

func NewSimpleEvaluator() *SimpleEvaluator { return &SimpleEvaluator{} }

func (SimpleEvaluator) EvalBool(ctx context.Context, expression string, env Vars) (bool, error) {
	v, err := SimpleEvaluator{}.EvalAny(ctx, expression, env)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, errf("expression result is %T, want bool", v)
	}
	return b, nil
}

// EvalAny 实现 ValueEvaluator：求值任意表达式（多实例 collection 等）。
func (SimpleEvaluator) EvalAny(_ context.Context, expression string, env Vars) (any, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("%w: empty expression", ErrExpression)
	}
	toks, terr := tokenize(expression)
	if terr != nil {
		return nil, terr
	}
	p := &exprParser{toks: toks}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tkEOF {
		return nil, errf("unexpected token %q at %d", p.cur().text, p.cur().pos)
	}
	return evalNode(node, env)
}

// ===================== 词法 =====================

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkNum
	tkStr
	tkIdent
	tkOp
)

type token struct {
	kind tokenKind
	text string
	val  any
	pos  int
}

var errExpr = ErrExpression

func errf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errExpr, fmt.Sprintf(format, args...))
}

func tokenize(src string) ([]token, error) {
	var toks []token
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c >= '0' && c <= '9':
			j := i
			for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, errf("bad number %q at %d", src[i:j], i)
			}
			toks = append(toks, token{tkNum, src[i:j], f, i})
			i = j
		case c == '\'' || c == '"':
			quote := c
			var sb strings.Builder
			j := i + 1
			for j < n {
				if src[j] == '\\' && j+1 < n {
					sb.WriteByte(src[j+1])
					j += 2
					continue
				}
				if src[j] == quote {
					break
				}
				sb.WriteByte(src[j])
				j++
			}
			if j >= n {
				return nil, errf("unterminated string at %d", i)
			}
			toks = append(toks, token{tkStr, src[i : j+1], sb.String(), i})
			i = j + 1
		case isIdentStart(c):
			j := i
			for j < n && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, token{tkIdent, src[i:j], nil, i})
			i = j
		default:
			two := ""
			if i+1 < n {
				two = src[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				toks = append(toks, token{tkOp, two, nil, i})
				i += 2
				continue
			}
			switch c {
			case '<', '>', '+', '-', '*', '/', '%', '!', '(', ')', '[', ']', ',', '.':
				toks = append(toks, token{tkOp, string(c), nil, i})
				i++
			default:
				return nil, errf("unexpected character %q at %d", string(c), i)
			}
		}
	}
	toks = append(toks, token{tkEOF, "", nil, n})
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

// ===================== AST =====================

type exprNode interface{}

type litNode struct{ val any }

type identNode struct{ path []string }

type unaryNode struct {
	op string
	x  exprNode
}

type binNode struct {
	op   string
	l, r exprNode
}

type arrNode struct{ items []exprNode }

// ===================== 语法 =====================

type exprParser struct {
	toks []token
	idx  int
}

func (p *exprParser) cur() token  { return p.toks[p.idx] }
func (p *exprParser) next() token { t := p.toks[p.idx]; p.idx++; return t }

func (p *exprParser) acceptOp(ops ...string) (string, bool) {
	t := p.cur()
	if t.kind != tkOp {
		return "", false
	}
	for _, op := range ops {
		if t.text == op {
			p.idx++
			return op, true
		}
	}
	return "", false
}

func (p *exprParser) parseExpr() (exprNode, error) { return p.parseOr() }

func (p *exprParser) parseOr() (exprNode, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.acceptOp("||"); !ok {
			return l, nil
		}
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = &binNode{"||", l, r}
	}
}

func (p *exprParser) parseAnd() (exprNode, error) {
	l, err := p.parseEq()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.acceptOp("&&"); !ok {
			return l, nil
		}
		r, err := p.parseEq()
		if err != nil {
			return nil, err
		}
		l = &binNode{"&&", l, r}
	}
}

func (p *exprParser) parseEq() (exprNode, error) {
	l, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("==", "!=")
		if !ok {
			return l, nil
		}
		r, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		l = &binNode{op, l, r}
	}
}

func (p *exprParser) parseCmp() (exprNode, error) {
	l, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	if op, ok := p.acceptOp("<", "<=", ">", ">="); ok {
		r, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &binNode{op, l, r}, nil
	}
	// in 操作符
	if t := p.cur(); t.kind == tkIdent && t.text == "in" {
		p.idx++
		r, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &binNode{"in", l, r}, nil
	}
	return l, nil
}

func (p *exprParser) parseAdd() (exprNode, error) {
	l, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("+", "-")
		if !ok {
			return l, nil
		}
		r, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		l = &binNode{op, l, r}
	}
}

func (p *exprParser) parseMul() (exprNode, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.acceptOp("*", "/", "%")
		if !ok {
			return l, nil
		}
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l = &binNode{op, l, r}
	}
}

func (p *exprParser) parseUnary() (exprNode, error) {
	if op, ok := p.acceptOp("!", "-"); ok {
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryNode{op, x}, nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (exprNode, error) {
	t := p.cur()
	switch {
	case t.kind == tkNum:
		p.idx++
		return &litNode{t.val}, nil
	case t.kind == tkStr:
		p.idx++
		return &litNode{t.val}, nil
	case t.kind == tkIdent:
		switch t.text {
		case "true":
			p.idx++
			return &litNode{true}, nil
		case "false":
			p.idx++
			return &litNode{false}, nil
		case "nil", "null":
			p.idx++
			return &litNode{nil}, nil
		}
		p.idx++
		path := []string{t.text}
		for {
			if _, ok := p.acceptOp("."); !ok {
				break
			}
			dot := p.cur()
			if dot.kind != tkIdent {
				return nil, errf("expected identifier after '.' at %d", dot.pos)
			}
			p.idx++
			path = append(path, dot.text)
		}
		return &identNode{path}, nil
	case t.kind == tkOp && t.text == "(":
		p.idx++
		x, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, ok := p.acceptOp(")"); !ok {
			return nil, errf("missing ')' at %d", p.cur().pos)
		}
		return x, nil
	case t.kind == tkOp && t.text == "[":
		p.idx++
		var items []exprNode
		if _, ok := p.acceptOp("]"); ok {
			return &arrNode{items}, nil
		}
		for {
			item, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			if _, ok := p.acceptOp(","); !ok {
				break
			}
		}
		if _, ok := p.acceptOp("]"); !ok {
			return nil, errf("missing ']' at %d", p.cur().pos)
		}
		return &arrNode{items}, nil
	default:
		return nil, errf("unexpected token %q at %d", t.text, t.pos)
	}
}

// ===================== 求值 =====================

func evalNode(n exprNode, env Vars) (any, error) {
	switch node := n.(type) {
	case *litNode:
		return node.val, nil
	case *identNode:
		return lookupPath(env, node.path), nil
	case *arrNode:
		items := make([]any, 0, len(node.items))
		for _, it := range node.items {
			v, err := evalNode(it, env)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return items, nil
	case *unaryNode:
		v, err := evalNode(node.x, env)
		if err != nil {
			return nil, err
		}
		switch node.op {
		case "!":
			b, ok := v.(bool)
			if !ok {
				return nil, errf("'!' requires bool operand, got %T", v)
			}
			return !b, nil
		case "-":
			f, ok := toNumber(v)
			if !ok {
				return nil, errf("'-' requires number operand, got %T", v)
			}
			return -f, nil
		}
	case *binNode:
		return evalBin(node, env)
	}
	return nil, errf("unsupported node")
}

func evalBin(node *binNode, env Vars) (any, error) {
	// 短路：&& / || 只在布尔域内运算。
	if node.op == "&&" || node.op == "||" {
		l, err := evalNode(node.l, env)
		if err != nil {
			return nil, err
		}
		lb, ok := l.(bool)
		if !ok {
			return nil, errf("'%s' requires bool operands, got %T", node.op, l)
		}
		if node.op == "&&" && !lb {
			return false, nil
		}
		if node.op == "||" && lb {
			return true, nil
		}
		r, err := evalNode(node.r, env)
		if err != nil {
			return nil, err
		}
		rb, ok := r.(bool)
		if !ok {
			return nil, errf("'%s' requires bool operands, got %T", node.op, r)
		}
		return rb, nil
	}

	l, err := evalNode(node.l, env)
	if err != nil {
		return nil, err
	}
	r, err := evalNode(node.r, env)
	if err != nil {
		return nil, err
	}

	switch node.op {
	case "==":
		return valueEqual(l, r), nil
	case "!=":
		return !valueEqual(l, r), nil
	case "in":
		arr, ok := r.([]any)
		if !ok {
			return nil, errf("'in' requires array on right, got %T", r)
		}
		for _, item := range arr {
			if valueEqual(l, item) {
				return true, nil
			}
		}
		return false, nil
	}

	// 数值/字符串运算与比较。
	ln, lok := toNumber(l)
	rn, rok := toNumber(r)
	switch node.op {
	case "<", "<=", ">", ">=":
		if lok && rok {
			return compareOrdered(ln, rn, node.op), nil
		}
		ls, lss := l.(string)
		rs, rss := r.(string)
		if lss && rss {
			return compareOrderedString(ls, rs, node.op), nil
		}
		return nil, errf("cannot compare %T and %T", l, r)
	case "+":
		if lok && rok {
			return ln + rn, nil
		}
		ls, lss := l.(string)
		rs, rss := r.(string)
		if lss && rss {
			return ls + rs, nil
		}
		return nil, errf("cannot add %T and %T", l, r)
	case "-", "*", "/", "%":
		if !lok || !rok {
			return nil, errf("'%s' requires number operands, got %T and %T", node.op, l, r)
		}
		switch node.op {
		case "-":
			return ln - rn, nil
		case "*":
			return ln * rn, nil
		case "/":
			if rn == 0 {
				return nil, errf("division by zero")
			}
			return ln / rn, nil
		case "%":
			if int64(rn) == 0 {
				return nil, errf("modulo by zero")
			}
			return float64(int64(ln) % int64(rn)), nil
		}
	}
	return nil, errf("unsupported operator %q", node.op)
}

func compareOrdered(l, r float64, op string) bool {
	switch op {
	case "<":
		return l < r
	case "<=":
		return l <= r
	case ">":
		return l > r
	case ">=":
		return l >= r
	}
	return false
}

func compareOrderedString(l, r, op string) bool {
	switch op {
	case "<":
		return l < r
	case "<=":
		return l <= r
	case ">":
		return l > r
	case ">=":
		return l >= r
	}
	return false
}

func toNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case bool:
		return 0, false
	}
	return 0, false
}

func valueEqual(l, r any) bool {
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	if lb, ok := l.(bool); ok {
		rb, ok2 := r.(bool)
		return ok2 && lb == rb
	}
	if _, ok := l.(string); ok {
		rs, ok2 := r.(string)
		if !ok2 {
			return false
		}
		return l.(string) == rs
	}
	if ln, ok := toNumber(l); ok {
		if rn, ok2 := toNumber(r); ok2 {
			return ln == rn
		}
		return false
	}
	return false
}

func lookupPath(env Vars, path []string) any {
	var cur any = map[string]any(env)
	for _, seg := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			vm, ok2 := cur.(Vars)
			if !ok2 {
				return nil
			}
			m = map[string]any(vm)
		}
		cur = m[seg]
	}
	return cur
}
