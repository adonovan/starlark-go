package resolve

import (
	"log"
	"strings"
	"fmt"
	"go.starlark.net/syntax"
)

// type checks a type (not term) expression.
// TODO: need a syntax for function types, structs.
//
// type = IDENT
//      | {type: type}
//      | [type]
//      | (type, ..., type)
//      | (type)
//      .

// type/term ambiguity:
// - int(x) conversion vs int("foo") call
// - {int: int} -- dict type, or singleton dict with function k/v?
// - [int] -- ditto list
// - (int,) -- ditto tuple
//
// what about dict(K, V), list(E), tuple(T, T, T). Would that help?
// what if we made the built-in functions into conversions, not funcs?
// structs and funcs
// - FUNC tuple tuple

type Type interface {
}

// {K: V}
type dictType struct {
	K, V Type
}

func (d *dictType) String() string {
	return fmt.Sprintf("{%s: %s}", d.K, d.V)
}

// [E]
type listType struct {
	E Type
}

func (l *listType) String() string {
	return fmt.Sprintf("[%s]", l.E)
}

// (T, ..., T)
type tupleType struct {
	T []Type
}

func (t *tupleType) String() string {
	var out strings.Builder	
	out.WriteByte('(')
	for i, elem := range t.T {
		if i > 0 {
			out.WriteString(", ")
		}
		fmt.Fprint(&out, elem)
	}
	if len(t.T) == 1 {
		out.WriteByte(',')
	}
	out.WriteByte(')')
	return out.String()
}

// reference to a type param, or to string, int, float, bytes, etc
// TODO atom? Nope, some of these have methods.
type identType struct {
	Name string //???
}

func (id *identType) String() string {
	return id.Name
}

var (
	stringType = identType{"string"}
	bytesType = identType{"bytes"} // TODO abolish
	boolType = identType{"bool"}
	intType = identType{"int"}
	floatType = identType{"float"}
	noneType = identType{"NoneType"}
	anyType = identType{"any"} // e.g. element type of [] list
	badType = identType{"bad"}
)

// TODO: func type
// TODO: merge this pass with resolver.

type checker struct {
	// Lexically first binding determines the type.
	env map[*Binding]Type
}

func typecheckFile(file *syntax.File) {
	ch := &checker{
		env: make(map[*Binding]Type),
	}
	ch.stmts(file.Stmts)
}

func (ch *checker) stmts(stmts []syntax.Stmt) {
	for _, s := range stmts {
		ch.stmt(s)
	}
}

func (ch *checker) bind(b *Binding, t Type) {
	fmt.Printf("%s: bind %s to %v\n", b.First.NamePos, b.First.Name, t)
	ch.env[b] = t // what if bound already?
}

type operand struct {
	T    Type
	term bool // true => denotes value, false => denotes type
	// other possibilities: denotes compile-time constant.
}

func (ch *checker) assign(lhs syntax.Expr, tRHS Type) {
	switch lhs := lhs.(type) {
	case *syntax.Ident:
		// x = ...
		ch.bind(lhs.Binding.(*Binding), tRHS)

	case *syntax.IndexExpr:
		// x[i] = ...
		tX := ch.term(lhs.X)
		tY := ch.term(lhs.Y)
		// TODO check x is indexable by y.
		// two cases: ([T])[int] or ({K: V])[K].  + user defined?
		// TODO: check elements of x are assignable from rhs.
		_ = tX
		_ = tY
		
	case *syntax.DotExpr:
		// x.f = ...
		tX := ch.expr(lhs.X)
		// TODO check tX has field X.
		// TODO check tX .X may be assigned from rRHS
		_ = tX
		
	case *syntax.TupleExpr:
		// (x, y) = ...
		// TODO check tRHS decomposes into elements
		for _, elem := range lhs.List {
			ch.assign(elem, tRHS) 
		}

	case *syntax.ListExpr:
		// [x, y, z] = ...
		// TODO check tRHS decomposes into elements
		for _, elem := range lhs.List {
			ch.assign(elem, tRHS)
		}

	case *syntax.ParenExpr:
		ch.assign(lhs.X, tRHS)
	}
}

func (ch *checker) stmt(s syntax.Stmt) {
	// TODO order of effects
	switch s := s.(type) {
	case *syntax.AssignStmt:
		// if op, then RHS must be scalar
		tRHS := ch.expr(s.RHS)
		ch.assign(s.LHS, tRHS)
		
	case *syntax.ExprStmt:
		ch.expr(s.X)
		
	case *syntax.ForStmt:	
		tX := ch.expr(s.X)
		ch.assign(s.Vars, tX) // TODO check iterable
		ch.stmts(s.Body)
		
	case *syntax.WhileStmt:
		ch.expr(s.Cond) // TODO check bool		
		ch.stmts(s.Body)
		
	case *syntax.IfStmt:
		ch.expr(s.Cond) // TODO check bool
		ch.stmts(s.True)
		ch.stmts(s.False)
		
	case *syntax.LoadStmt:
		// get types from loaded package and bind.
		panic("load not yet implemented")
		
	case *syntax.ReturnStmt:
		if s.Result != nil {
			ch.expr(s.Result)
		}
	}
}

func (ch *checker) typeExpr(e syntax.Expr) Type {
	switch e := e.(type) {
	case *syntax.DictExpr:

	case *syntax.Ident:
		// TODO check predeclared
		switch e.Name {
		case "string":
			return &stringType
		case "bytes":
			return &bytesType
		case "bool":
			return &boolType
		case "int":
			return &intType
		case "float":
			return &floatType
		}
		t := ch.env[e.Binding.(*Binding)]
		if t == nil {
			log.Fatal("ident is not a type")
		}
		return t
		
	case *syntax.ListExpr:
		if len(e.List) != 1 {
			log.Fatal("list type requires one element")
		}
		tElem := ch.typeExpr(e.List[0])
		return &listType{tElem}
				
	case *syntax.ParenExpr:
		return ch.typeExpr(e.X)
		
	case *syntax.TupleExpr:
		tElems := make([]Type, len(e.List))
		for i, elem := range e.List{
			tElems[i] = ch.typeExpr(elem)
		}
		return &tupleType{tElems}
	}
	log.Print("not a type")
	return &badType
}

// term type checks an expression that denotes a value.
func (ch *checker) term(e syntax.Expr) Type {
	op := ch.expr(e)
	if !op.term {
		log.Print("type, not term")
	}
	return op.T
}

func (ch *checker) expr(e syntax.Expr) Type {
	switch e := e.(type) {
	case *syntax.BinaryExpr:
		tX := ch.expr(e.X)
		tY := ch.expr(e.Y)
		switch e.Op {
		case syntax.PLUS:
			// both operands must have same type
			if tX != tY { // TODO us eq not ==
				fmt.Println(tX, e.Op, tY)		
			}
		}
		return tX // FIXME
		
	case *syntax.CallExpr: // call or conv
		
	case *syntax.Comprehension:
		for _, clause := range e.Clauses {
			switch clause := clause.(type) {
			case *syntax.ForClause:
				tX := ch.expr(clause.X)
				// TODO check tX is iterable
				ch.assign(clause.Vars, tX)
			case *syntax.IfClause:
				tCond := ch.expr(clause.Cond)
				_ = tCond // TODO check bool
			}
		}
		if e.Curly {
			entry := e.Body.(*syntax.DictEntry)
			tK := ch.expr(entry.Key) 
			tV := ch.expr(entry.Value)
			// TODO check tK is comparable
			return &dictType{tK, tV}
		} else {
			tBody := ch.expr(e.Body)
			return &listType{tBody}
		}
				
	case *syntax.CondExpr:
		tCond := ch.expr(e.Cond)
		_ = tCond // TODO check bool
		tTrue := ch.expr(e.True)
		tFalse := ch.expr(e.False)
		_ = tFalse
		// TODO check tTrue that tFalse are mutually assignable
		// in at least one direction, and return supertype.
		// Needs principle types.
		
		return tTrue
		
	case *syntax.DictExpr:
		// dict type {Kgi: V} or term {k: v, ...}.
		if len(e.List) == 0 {
			return &dictType{anyType, anyType} // empty dict
		}
		for i, e := range e.List {
			entry := e.(*syntax.DictEntry)
			k := ch.expr(e.Key)
			// TODO check key type is comparable
			v := ch.expr(e.Value)
			if len(e.List) == 1 && !k.term && v.term {
				// dict type
				return &dictType{k.T, v.T}
			} else {
				// dict term
				if !k.term || !v.term {
					log.Fatalf("dict k/v type is not a term")					
				}
				// TODOs check all keys and values conform
				// and return 
			}
		}
		return &dictType{}
		
		
	case *syntax.DotExpr:
		log.Fatal("DotExpr")
		// FIXME what's the type of a struct???
		// TODO  Methods of built-ins
		// user-defined attrs
		
	case *syntax.Ident:
		return ch.env[e.Binding.(*Binding)]
		
	case *syntax.IndexExpr:
		tX := ch.expr(e.X)
		_ = tX
		tY := ch.expr(e.Y)
		_ = tY
		// TODO check X is indexable by Y   (list[int], dict[key])
		// and return element type.
		return &badType // FIXME
		
	case *syntax.LambdaExpr:
		return &badType // FIXME
		
	case *syntax.ListExpr:
		if len(e.List) != 1 {
			log.Fatal("list type requires one element")
		}
		tElem := ch.expr(e.List[0])
		return &listType{tElem}
		
	case *syntax.Literal:
		switch e.Token {
		case syntax.STRING:
			return &stringType
		case syntax.BYTES:
			return &bytesType
		case syntax.INT:
			return &intType
		case syntax.FLOAT:
			return &floatType
		}
		panic("oops")
		
	case *syntax.ParenExpr:
		return ch.expr(e.X)
		
	case *syntax.SliceExpr:
		tX := ch.expr(e.X) // TODO check sliceable
		if e.Lo != nil {
			_ = ch.expr(e.Lo) // TODO check int
		}
		if e.Hi != nil {
			_ = ch.expr(e.Hi) // TODO check int
		}
		if e.Step != nil {
			_ = ch.expr(e.Step) // TODO check int
		}
		return tX
		
	case *syntax.TupleExpr:
		tElems := make([]Type, len(e.List))
		for i, elem := range e.List{
			tElems[i] = ch.expr(elem)
		}
		return &tupleType{tElems}
		
	case *syntax.UnaryExpr:
		// Unary operators must not change the type.
		// TODO is it possible to relax that with type plugins?
		return ch.expr(e.X)
	}
	return &badType
}


/*

   syntax lambda/def params, 

   
# parametric polymorphism in an interpreted language.



func new[T]() T {
   var x T // T.zeroval()
   return x
}

func max[T](x, y T) {
   if x < y { // T.<
      return y
   } else {
      return x
   }
}

func maxAll[T](slice []T) {
  var m T
  foreach x in slice {  // T.iterate
     m = max[T](x, m)
  }
  return m
}

It's a bit like an interface value, but for operators not methods.
Syntax overloading is tricky (e.g. ,ok) because we know nothing.


x<y compiles to int < int if both args are int, or to T.< otherwise.
Nonparameterized code should be efficient. Parameterized is like
interface code in Go, though it could be jitted or expanded.
(Like PyPy??)

type checker must find



type rules for exprs + stmts:
ident, literal, paren = easy
dict = {k, v....} needs supertype over all k, all v
list = [e, ...] ditto
unary =
  +, -, ~ -> int or float.   T->T
  not -> bool
binary
  X and/or Y -> bool?
  ==   !=   <    >   <=   >=   in   not in    -> bool
  all others are T+T=T    + - * / // % << >> | ^ &
et if cond else ef -> join(et, ef)
[e for x in y] -> same as 'for x in y: list.add(e)'
y = f(a, b)
x.f
a[i]  a[i::]
lambda/def
x, y = expr

Type(expr) -- conversion


are interfaces needed?


types: these names are predeclared to type objects:
Are types values??

string bytes int float - predeclared
[T] -- parsing ambiguity. an expr.  (T)(expr) is then a fun call
(T, ... Tn) -- ditto
{K: V} -- ditto

So, the [] () {} constructors work as either types and terms, but not both.
Are they dependent types?

Q. instanceof?
   type constraints?
   conversions are T(expr) call syntax.
   union type?

grammar: same as starlark, except:

- functions have types, and type parameters
    def f[T](x T, y string, z [T], a (T, string, int)])
- 



what about user-defined struct types??




arrays? sized ints? why not go for efficiency.


func sort[T](elems []T) {
   if n == 2 {
      if elems[0] < elems[1] {  // T.<
         swap(elems[0:1]) 
      }
   }
   sort[T](elems[:n/2])
   sort[T](elems[n/2:])
   merge[T](elems)
}

*/
