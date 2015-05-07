// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// item represents a token or text string returned from the scanner.
type item struct {
	typ itemType // The type of this item.
	pos Pos      // The starting position, in bytes, of this item in the input string.
	val string   // The value of this item.
}

// String returns a descriptive string for the item.
func (i item) String() string {
	switch {
	case i.typ == itemEOF:
		return "EOF"
	case i.typ == itemError:
		return i.val
	case i.typ == itemIdentifier || i.typ == itemMetricIdentifier:
		return fmt.Sprintf("%q", i.val)
	case i.typ.isKeyword():
		return fmt.Sprintf("<%s>", i.val)
	case i.typ.isOperator():
		return fmt.Sprintf("<op:%s>", i.val)
	case i.typ.isAggregator():
		return fmt.Sprintf("<aggr:%s>", i.val)
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

// isOperator returns true if the item corresponds to a logical or arithmetic operator.
// Returns false otherwise.
func (i itemType) isOperator() bool { return i > operatorsStart && i < operatorsEnd }

// isAggregator returns true if the item belongs to the aggregator functions.
// Returns false otherwise
func (i itemType) isAggregator() bool { return i > aggregatorsStart && i < aggregatorsEnd }

// isKeyword returns true if the item corresponds to a keyword.
// Returns false otherwise.
func (i itemType) isKeyword() bool { return i > keywordsStart && i < keywordsEnd }

// Constants for operator precedence in expressions.
//
const LowestPrec = 0 // Non-operators.

// Precedence returns the operator precedence of the binary
// operator op. If op is not a binary operator, the result
// is LowestPrec.
func (i itemType) precedence() int {
	switch i {
	case itemLOR:
		return 1
	case itemLAND:
		return 2
	case itemEQL, itemNEQ, itemLTE, itemLSS, itemGTE, itemGTR:
		return 3
	case itemADD, itemSUB:
		return 4
	case itemMUL, itemDIV, itemMOD:
		return 5
	default:
		return LowestPrec
	}
}

type itemType int

const (
	itemError itemType = iota // Error occurred, value is error message
	itemEOF
	itemComment
	itemIdentifier
	itemMetricIdentifier
	itemLeftParen
	itemRightParen
	itemLeftBrace
	itemRightBrace
	itemLeftBracket
	itemRightBracket
	itemComma
	itemAssign
	itemSemicolon
	itemString
	itemNumber
	itemDuration

	operatorsStart
	// Operators.
	itemSUB
	itemADD
	itemMUL
	itemMOD
	itemDIV
	itemLAND
	itemLOR
	itemEQL
	itemNEQ
	itemLTE
	itemLSS
	itemGTE
	itemGTR
	itemEQLRegex
	itemNEQRegex
	operatorsEnd

	aggregatorsStart
	// Aggregators.
	itemAvg
	itemCount
	itemSum
	itemMin
	itemMax
	itemStddev
	itemStdvar
	aggregatorsEnd

	keywordsStart
	// Keywords.
	itemAlert
	itemIf
	itemFor
	itemWith
	itemSummary
	itemDescription
	itemKeepingExtra
	itemOffset
	itemBy
	itemOn
	itemGroupLeft
	itemGroupRight
	keywordsEnd
)

var key = map[string]itemType{
	// Operators.
	"and": itemLAND,
	"or":  itemLOR,

	// Aggregators.
	"sum":    itemSum,
	"avg":    itemAvg,
	"count":  itemCount,
	"min":    itemMin,
	"max":    itemMax,
	"stddev": itemStddev,
	"stdvar": itemStdvar,

	// Keywords.
	"alert":         itemAlert,
	"if":            itemIf,
	"for":           itemFor,
	"with":          itemWith,
	"summary":       itemSummary,
	"description":   itemDescription,
	"offset":        itemOffset,
	"by":            itemBy,
	"keeping_extra": itemKeepingExtra,
	"on":            itemOn,
	"group_left":    itemGroupLeft,
	"group_right":   itemGroupRight,
}

// These are the default string representations for common items. It does not
// imply that those are the only character sequences that can be lexed to such an item.
var itemTypeStr = map[itemType]string{
	itemLeftParen:    "(",
	itemRightParen:   ")",
	itemLeftBrace:    "{",
	itemRightBrace:   "}",
	itemLeftBracket:  "[",
	itemRightBracket: "]",
	itemComma:        ",",
	itemAssign:       "=",
	itemSemicolon:    ";",

	itemSUB:      "-",
	itemADD:      "+",
	itemMUL:      "*",
	itemMOD:      "%",
	itemDIV:      "/",
	itemEQL:      "==",
	itemNEQ:      "!=",
	itemLTE:      "<=",
	itemLSS:      "<",
	itemGTE:      ">=",
	itemGTR:      ">",
	itemEQLRegex: "=~",
	itemNEQRegex: "!~",
}

func init() {
	// Add keywords to item type strings.
	for s, ty := range key {
		itemTypeStr[ty] = s
	}
}

func (t itemType) String() string {
	if s, ok := itemTypeStr[t]; ok {
		return s
	}
	return fmt.Sprintf("<item %d>", t)
}

func (i item) desc() string {
	if _, ok := itemTypeStr[i.typ]; ok {
		return i.String()
	}
	if i.typ == itemEOF {
		return i.typ.desc()
	}
	return fmt.Sprintf("%s %s", i.typ.desc(), i)
}

func (t itemType) desc() string {
	switch t {
	case itemError:
		return "error"
	case itemEOF:
		return "end of input"
	case itemComment:
		return "comment"
	case itemIdentifier:
		return "identifier"
	case itemMetricIdentifier:
		return "metric identifier"
	case itemString:
		return "string"
	case itemNumber:
		return "number"
	case itemDuration:
		return "duration"
	}
	return fmt.Sprintf("%q", t)
}

const eof = -1

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*lexer) stateFn

// Pos is the position in a string.
type Pos int

// lexer holds the state of the scanner.
type lexer struct {
	input   string    // The string being scanned.
	state   stateFn   // The next lexing function to enter.
	pos     Pos       // Current position in the input.
	start   Pos       // Start position of this item.
	width   Pos       // Width of last rune read from input.
	lastPos Pos       // Position of most recent item returned by nextItem.
	items   chan item // Channel of scanned items.

	parenDepth  int  // Nesting depth of ( ) exprs.
	braceOpen   bool // Whether a { is opened.
	bracketOpen bool // Whether a [ is opened.
	stringOpen  rune // Quote rune of the string currently being read.
}

// next returns the next rune in the input.
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
}

// emit passes an item back to the client.
func (l *lexer) emit(t itemType) {
	l.items <- item{t, l.start, l.input[l.start:l.pos]}
	l.start = l.pos
}

// ignore skips over the pending input before this point.
func (l *lexer) ignore() {
	l.start = l.pos
}

// accept consumes the next rune if it's from the valid set.
func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes from the valid set.
func (l *lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
		// consume
	}
	l.backup()
}

// lineNumber reports which line we're on, based on the position of
// the previous item returned by nextItem. Doing it this way
// means we don't have to worry about peek double counting.
func (l *lexer) lineNumber() int {
	return 1 + strings.Count(l.input[:l.lastPos], "\n")
}

// linePosition reports at which character in the current line
// we are on.
func (l *lexer) linePosition() int {
	lb := strings.LastIndex(l.input[:l.lastPos], "\n")
	if lb == -1 {
		return 1 + int(l.lastPos)
	}
	return 1 + int(l.lastPos) - lb
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{itemError, l.start, fmt.Sprintf(format, args...)}
	return nil
}

// nextItem returns the next item from the input.
func (l *lexer) nextItem() item {
	item := <-l.items
	l.lastPos = item.pos
	return item
}

// lex creates a new scanner for the input string.
func lex(input string) *lexer {
	l := &lexer{
		input: input,
		items: make(chan item),
	}
	go l.run()
	return l
}

// run runs the state machine for the lexer.
func (l *lexer) run() {
	for l.state = lexStatements; l.state != nil; {
		l.state = l.state(l)
	}
	close(l.items)
}

// lineComment is the character that starts a line comment.
const lineComment = "#"

// lexStatements is the top-level state for lexing.
func lexStatements(l *lexer) stateFn {
	if l.braceOpen {
		return lexInsideBraces
	}
	if strings.HasPrefix(l.input[l.pos:], lineComment) {
		return lexLineComment
	}

	switch r := l.next(); {
	case r == eof:
		if l.parenDepth != 0 {
			return l.errorf("unclosed left parenthesis")
		} else if l.bracketOpen {
			return l.errorf("unclosed left bracket")
		}
		l.emit(itemEOF)
		return nil
	case r == ',':
		l.emit(itemComma)
	case isSpace(r):
		return lexSpace
	case r == '*':
		l.emit(itemMUL)
	case r == '/':
		l.emit(itemDIV)
	case r == '%':
		l.emit(itemMOD)
	case r == '+':
		l.emit(itemADD)
	case r == '-':
		l.emit(itemSUB)
	case r == '=':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(itemEQL)
		} else if t == '~' {
			return l.errorf("unexpected character after '=': %q", t)
		} else {
			l.emit(itemAssign)
		}
	case r == '!':
		if t := l.next(); t == '=' {
			l.emit(itemNEQ)
		} else {
			return l.errorf("unexpected character after '!': %q", t)
		}
	case r == '<':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(itemLTE)
		} else {
			l.emit(itemLSS)
		}
	case r == '>':
		if t := l.peek(); t == '=' {
			l.next()
			l.emit(itemGTE)
		} else {
			l.emit(itemGTR)
		}
	case unicode.IsDigit(r) || (r == '.' && unicode.IsDigit(l.peek())):
		l.backup()
		return lexNumberOrDuration
	case r == '"' || r == '\'':
		l.stringOpen = r
		return lexString
	case r == 'N' || r == 'n' || r == 'I' || r == 'i':
		n2 := strings.ToLower(l.input[l.pos:])
		if len(n2) < 3 || !isAlphaNumeric(rune(n2[2])) {
			if (r == 'N' || r == 'n') && strings.HasPrefix(n2, "an") {
				l.pos += 2
				l.emit(itemNumber)
				break
			}
			if (r == 'I' || r == 'i') && strings.HasPrefix(n2, "nf") {
				l.pos += 2
				l.emit(itemNumber)
				break
			}
		}
		fallthrough
	case isAlphaNumeric(r) || r == ':':
		l.backup()
		return lexKeywordOrIdentifier
	case r == '(':
		l.emit(itemLeftParen)
		l.parenDepth++
		return lexStatements
	case r == ')':
		l.emit(itemRightParen)
		l.parenDepth--
		if l.parenDepth < 0 {
			return l.errorf("unexpected right parenthesis %q", r)
		}
		return lexStatements
	case r == '{':
		l.emit(itemLeftBrace)
		l.braceOpen = true
		return lexInsideBraces(l)
	case r == '[':
		if l.bracketOpen {
			return l.errorf("unexpected left bracket %q", r)
		}
		l.emit(itemLeftBracket)
		l.bracketOpen = true
		return lexDuration
	case r == ']':
		if !l.bracketOpen {
			return l.errorf("unexpected right bracket %q", r)
		}
		l.emit(itemRightBracket)
		l.bracketOpen = false

	default:
		return l.errorf("unexpected character: %q", r)
	}
	return lexStatements
}

// lexInsideBraces scans the inside of a vector selector. Keywords are ignored and
// scanned as identifiers.
func lexInsideBraces(l *lexer) stateFn {
	if strings.HasPrefix(l.input[l.pos:], lineComment) {
		return lexLineComment
	}

	switch r := l.next(); {
	case r == eof:
		return l.errorf("unexpected end of input inside braces")
	case isSpace(r):
		return lexSpace
	case unicode.IsLetter(r) || r == '_':
		l.backup()
		return lexIdentifier
	case r == ',':
		l.emit(itemComma)
	case r == '"' || r == '\'':
		l.stringOpen = r
		return lexString
	case r == '=':
		if l.next() == '~' {
			l.emit(itemEQLRegex)
			break
		}
		l.backup()
		l.emit(itemEQL)
	case r == '!':
		switch nr := l.next(); {
		case nr == '~':
			l.emit(itemNEQRegex)
		case nr == '=':
			l.emit(itemNEQ)
		default:
			return l.errorf("unexpected character after '!' inside braces: %q", nr)
		}
	case r == '{':
		return l.errorf("unexpected left brace %q", r)
	case r == '}':
		l.emit(itemRightBrace)
		l.braceOpen = false
		return lexStatements
	default:
		return l.errorf("unexpected character inside braces: %q", r)
	}
	return lexInsideBraces
}

// lexString scans a quoted string. The initial quote has already been seen.
func lexString(l *lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated quoted string")
		case l.stringOpen:
			break Loop
		}
	}
	l.emit(itemString)
	return lexStatements
}

// lexSpace scans a run of space characters. One space has already been seen.
func lexSpace(l *lexer) stateFn {
	for isSpace(l.peek()) {
		l.next()
	}
	l.ignore()
	return lexStatements
}

// lexLineComment scans a line comment. Left comment marker is known to be present.
func lexLineComment(l *lexer) stateFn {
	l.pos += Pos(len(lineComment))
	for r := l.next(); !isEndOfLine(r) && r != eof; {
		r = l.next()
	}
	l.backup()
	l.emit(itemComment)
	return lexStatements
}

func lexDuration(l *lexer) stateFn {
	if l.scanNumber() {
		return l.errorf("missing unit character in duration")
	}
	// Next two chars must be a valid unit and a non-alphanumeric.
	if l.accept("smhdwy") {
		if isAlphaNumeric(l.next()) {
			return l.errorf("bad duration syntax: %q", l.input[l.start:l.pos])
		}
		l.backup()
		l.emit(itemDuration)
		return lexStatements
	}
	return l.errorf("bad duration syntax: %q", l.input[l.start:l.pos])
}

// lexNumber scans a number: decimal, hex, oct or float.
func lexNumber(l *lexer) stateFn {
	if !l.scanNumber() {
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	}
	l.emit(itemNumber)
	return lexStatements
}

// lexNumberOrDuration scans a number or a duration item.
func lexNumberOrDuration(l *lexer) stateFn {
	if l.scanNumber() {
		l.emit(itemNumber)
		return lexStatements
	}
	// Next two chars must be a valid unit and a non-alphanumeric.
	if l.accept("smhdwy") {
		if isAlphaNumeric(l.next()) {
			return l.errorf("bad number or duration syntax: %q", l.input[l.start:l.pos])
		}
		l.backup()
		l.emit(itemDuration)
		return lexStatements
	}
	return l.errorf("bad number or duration syntax: %q", l.input[l.start:l.pos])
}

// scanNumber scans numbers of different formats. The scanned item is
// not necessarily a valid number. This case is caught by the parser.
func (l *lexer) scanNumber() bool {
	digits := "0123456789"
	if l.accept("0") && l.accept("xX") {
		digits = "0123456789abcdefABCDEF"
	}
	l.acceptRun(digits)
	if l.accept(".") {
		l.acceptRun(digits)
	}
	if l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789")
	}
	// Next thing must not be alphanumeric.
	if isAlphaNumeric(l.peek()) {
		return false
	}
	return true
}

// lexIdentifier scans an alphanumeric identifier. The next character
// is known to be a letter.
func lexIdentifier(l *lexer) stateFn {
	for isAlphaNumeric(l.next()) {
		// absorb
	}
	l.backup()
	l.emit(itemIdentifier)
	return lexStatements
}

// lexKeywordOrIdentifier scans an alphanumeric identifier which may contain
// a colon rune. If the identifier is a keyword the respective keyword item
// is scanned.
func lexKeywordOrIdentifier(l *lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || r == ':':
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if kw, ok := key[strings.ToLower(word)]; ok {
				l.emit(kw)
			} else if !strings.Contains(word, ":") {
				l.emit(itemIdentifier)
			} else {
				l.emit(itemMetricIdentifier)
			}
			break Loop
		}
	}
	return lexStatements
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

// isEndOfLine reports whether r is an end-of-line character.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || unicode.IsDigit(r)
}
