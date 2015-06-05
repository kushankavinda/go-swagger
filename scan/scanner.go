package scan

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"log"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/loader"

	"github.com/casualjim/go-swagger/spec"
	"github.com/casualjim/go-swagger/util"
)

const (
	rxMethod = "(\\p{L}+)"
	rxPath   = "((?:/[\\p{L}\\p{N}\\p{Pd}\\p{Pc}{}]*)+/?)"
	rxOpTags = "(\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}\\p{Zs}]+)"
	rxOpID   = "((?:\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}]+)+)"

	rxMaximumFmt    = "%s[Mm]ax(?:imum)?\\p{Zs}*:\\p{Zs}*([\\<=])?\\p{Zs}*([\\+-]?(?:\\p{N}+\\.)?\\p{N}+)$"
	rxMinimumFmt    = "%s[Mm]in(?:imum)?\\p{Zs}*:\\p{Zs}*([\\>=])?\\p{Zs}*([\\+-]?(?:\\p{N}+\\.)?\\p{N}+)$"
	rxMultipleOfFmt = "%s[Mm]ultiple\\p{Zs}*[Oo]f\\p{Zs}*:\\p{Zs}*([\\+-]?(?:\\p{N}+\\.)?\\p{N}+)$"

	rxMaxLengthFmt        = "%s[Mm]ax(?:imum)?(?:\\p{Zs}*[\\p{Pd}\\p{Pc}]?[Ll]en(?:gth)?)\\p{Zs}*:\\p{Zs}*(\\p{N}+)$"
	rxMinLengthFmt        = "%s[Mm]in(?:imum)?(?:\\p{Zs}*[\\p{Pd}\\p{Pc}]?[Ll]en(?:gth)?)\\p{Zs}*:\\p{Zs}*(\\p{N}+)$"
	rxPatternFmt          = "%s[Pp]attern\\p{Zs}*:\\p{Zs}*(.*)$"
	rxCollectionFormatFmt = "%s[Cc]ollection(?:\\p{Zs}*[\\p{Pd}\\p{Pc}]?[Ff]ormat)\\p{Zs}*:\\p{Zs}*(.*)$"

	rxMaxItemsFmt = "%s[Mm]ax(?:imum)?(?:\\p{Zs}*|[\\p{Pd}\\p{Pc}]|\\.)?[Ii]tems\\p{Zs}*:\\p{Zs}*(\\p{N}+)$"
	rxMinItemsFmt = "%s[Mm]in(?:imum)?(?:\\p{Zs}*|[\\p{Pd}\\p{Pc}]|\\.)?[Ii]tems\\p{Zs}*:\\p{Zs}*(\\p{N}+)$"
	rxUniqueFmt   = "%s[Uu]nique\\p{Zs}*:\\p{Zs}*(true|false)$"

	rxItemsPrefix = "(?:[Ii]tems[\\.\\p{Zs}]?)+"
)

var (
	rxSwaggerAnnotation  = regexp.MustCompile("[^+]*\\+\\p{Zs}*swagger:([\\p{L}\\p{N}\\p{Pd}\\p{Pc}]+)")
	rxMeta               = regexp.MustCompile("\\+swagger:meta")
	rxStrFmt             = regexp.MustCompile("\\+swagger:strfmt\\p{Zs}*(\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}]+)$")
	rxModelOverride      = regexp.MustCompile("\\+swagger:model\\p{Zs}*(\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}]+)?$")
	rxResponseOverride   = regexp.MustCompile("\\+swagger:response\\p{Zs}*(\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}]+)?$")
	rxParametersOverride = regexp.MustCompile("\\+swagger:parameters\\p{Zs}*(\\p{L}[\\p{L}\\p{N}\\p{Pd}\\p{Pc}\\p{Zs}]+)$")
	rxRoute              = regexp.MustCompile(
		"\\+swagger:route\\p{Zs}*" +
			rxMethod +
			"\\p{Zs}*" +
			rxPath +
			"\\p{Zs}+" +
			rxOpTags +
			"\\p{Zs}+" +
			rxOpID + "$")

	rxIn                 = regexp.MustCompile("(?:[Ii]n|[Ss]ource)\\p{Zs}*:\\p{Zs}*(query|path|header|body)$")
	rxRequired           = regexp.MustCompile("[Rr]equired\\p{Zs}*:\\p{Zs}*(true|false)$")
	rxReadOnly           = regexp.MustCompile("[Rr]ead(?:\\p{Zs}*|[\\p{Pd}\\p{Pc}])?[Oo]nly\\p{Zs}*:\\p{Zs}*(true|false)$")
	rxSpace              = regexp.MustCompile("\\p{Zs}+")
	rxNotAlNumSpaceComma = regexp.MustCompile("[^\\p{L}\\p{N}\\p{Zs},]")
	rxPunctuationEnd     = regexp.MustCompile("\\p{Po}$")
	rxStripComments      = regexp.MustCompile("^[^\\w\\+]*")
	rxStripTitleComments = regexp.MustCompile("^[^\\p{L}]*[Pp]ackage\\p{Zs}+[^\\p{Zs}]+\\p{Zs}*")

	rxConsumes  = regexp.MustCompile("[Cc]onsumes\\p{Zs}*:")
	rxProduces  = regexp.MustCompile("[Pp]roduces\\p{Zs}*:")
	rxSecurity  = regexp.MustCompile("[Ss]ecurity\\p{Zs}*:")
	rxResponses = regexp.MustCompile("[Rr]esponses\\p{Zs}*:")
	rxSchemes   = regexp.MustCompile("[Ss]chemes\\p{Zs}*:\\p{Zs}*((?:(?:https?|HTTPS?|wss?|WSS?)[\\p{Zs},]*)+)$")
	rxVersion   = regexp.MustCompile("[Vv]ersion\\p{Zs}*:\\p{Zs}*(.+)$")
	rxHost      = regexp.MustCompile("[Hh]ost\\p{Zs}*:\\p{Zs}*(.+)$")
	rxBasePath  = regexp.MustCompile("[Bb]ase\\p{Zs}*-*[Pp]ath\\p{Zs}*:\\p{Zs}*" + rxPath + "$")
	rxLicense   = regexp.MustCompile("[Ll]icense\\p{Zs}*:\\p{Zs}*(.+)$")
	rxContact   = regexp.MustCompile("[Cc]ontact\\p{Zs}*-?(?:[Ii]info\\p{Zs}*)?:\\p{Zs}*(.+)$")
	rxTOS       = regexp.MustCompile("[Tt](:?erms)?\\p{Zs}*-?[Oo]f?\\p{Zs}*-?[Ss](?:ervice)?\\p{Zs}*:")
)

// Many thanks go to https://github.com/yvasiyarov/swagger
// this is loosely based on that implementation but for swagger 2.0

type setter func(interface{}, []string) error

func joinDropLast(lines []string) string {
	l := len(lines)
	lns := lines
	if l > 0 && len(strings.TrimSpace(lines[l-1])) == 0 {
		lns = lines[:l-1]
	}
	return strings.Join(lns, "\n")
}

func removeEmptyLines(lines []string) (notEmpty []string) {
	for _, l := range lines {
		if len(strings.TrimSpace(l)) > 0 {
			notEmpty = append(notEmpty, l)
		}
	}
	return
}

func rxf(rxp, ar string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(rxp, ar))
}

// Application scans the application and builds a swagger spec based on the information from the code files.
// When there are includes provided, only those files are considered for the initial discovery.
// Similarly the excludes will exclude an item from initial discovery through scanning for annotations.
// When something in the discovered items requires a type that is contained in the includes or excludes it will still be
// in the spec.
func Application(bp string, input *spec.Swagger, includes, excludes packageFilters) (*spec.Swagger, error) {
	parser, err := newAppScanner(bp, input, includes, excludes)
	if err != nil {
		return nil, err
	}
	return parser.Parse()
}

// appScanner the global context for scanning a go application
// into a swagger specification
type appScanner struct {
	loader      *loader.Config
	prog        *loader.Program
	classifier  *programClassifier
	discovered  []schemaDecl
	input       *spec.Swagger
	definitions map[string]spec.Schema
	responses   map[string]spec.Response
	operations  map[string]*spec.Operation

	// MainPackage the path to find the main class in
	MainPackage string
}

// newAPIParser creates a new api parser
func newAppScanner(bp string, input *spec.Swagger, includes, excludes packageFilters) (*appScanner, error) {
	var ldr loader.Config
	ldr.ParserMode = goparser.ParseComments
	ldr.Import(bp)
	prog, err := ldr.Load()
	if err != nil {
		return nil, err
	}
	if input == nil {
		input = new(spec.Swagger)
	}

	if input.Paths == nil {
		input.Paths = new(spec.Paths)
	}
	if input.Definitions == nil {
		input.Definitions = make(map[string]spec.Schema)
	}
	if input.Responses == nil {
		input.Responses = make(map[string]spec.Response)
	}

	return &appScanner{
		MainPackage: bp,
		prog:        prog,
		input:       input,
		loader:      &ldr,
		operations:  collectOperationsFromInput(input),
		definitions: input.Definitions,
		responses:   input.Responses,
		classifier: &programClassifier{
			Includes: includes,
			Excludes: excludes,
		},
	}, nil
}

func collectOperationsFromInput(input *spec.Swagger) map[string]*spec.Operation {
	operations := make(map[string]*spec.Operation)
	if input != nil && input.Paths != nil {
		for _, pth := range input.Paths.Paths {
			if pth.Get != nil {
				operations[pth.Get.ID] = pth.Get
			}
			if pth.Post != nil {
				operations[pth.Post.ID] = pth.Post
			}
			if pth.Put != nil {
				operations[pth.Put.ID] = pth.Put
			}
			if pth.Patch != nil {
				operations[pth.Patch.ID] = pth.Patch
			}
			if pth.Delete != nil {
				operations[pth.Delete.ID] = pth.Delete
			}
			if pth.Head != nil {
				operations[pth.Head.ID] = pth.Head
			}
			if pth.Options != nil {
				operations[pth.Options.ID] = pth.Options
			}
		}
	}
	return operations
}

// Parse produces a swagger object for an application
func (a *appScanner) Parse() (*spec.Swagger, error) {
	// classification still includes files that are completely commented out
	cp, err := a.classifier.Classify(a.prog)
	if err != nil {
		return nil, err
	}

	// build parameters dictionary
	for _, paramsFile := range cp.Parameters {
		if err := a.parseParameters(paramsFile); err != nil {
			return nil, err
		}
	}

	// build responses dictionary
	for _, responseFile := range cp.Responses {
		if err := a.parseResponses(responseFile); err != nil {
			return nil, err
		}
	}

	// build definitions dictionary
	if err := a.processDiscovered(); err != nil {
		return nil, err
	}

	// build paths dictionary
	for _, routeFile := range cp.Operations {
		if err := a.parseRoutes(routeFile); err != nil {
			return nil, err
		}
	}

	// build swagger object
	for _, metaFile := range cp.Meta {
		if err := a.parseMeta(metaFile); err != nil {
			return nil, err
		}
	}
	return a.input, nil
}

func (a *appScanner) processDiscovered() error {
	// loop over discovered until all the items are in definitions
	keepGoing := len(a.discovered) > 0
	for keepGoing {
		var queue []schemaDecl
		for _, d := range a.discovered {
			if _, ok := a.definitions[d.Name]; !ok {
				queue = append(queue, d)
			}
		}
		a.discovered = nil
		for _, sd := range queue {
			if err := a.parseSchema(sd.File); err != nil {
				return err
			}
		}
		keepGoing = len(a.discovered) > 0
	}

	return nil
}

func (a *appScanner) parseSchema(file *ast.File) error {
	sp := newSchemaParser(a.prog)
	if err := sp.Parse(file, a.definitions); err != nil {
		return err
	}
	a.discovered = append(a.discovered, sp.postDecls...)
	return nil
}

func (a *appScanner) parseRoutes(file *ast.File) error {
	rp := newRoutesParser(a.prog)
	rp.operations = a.operations
	rp.definitions = a.definitions
	rp.responses = a.responses
	if err := rp.Parse(file, a.input.Paths); err != nil {
		return err
	}
	return nil
}

func (a *appScanner) parseParameters(file *ast.File) error {
	rp := newParameterParser(a.prog)
	if err := rp.Parse(file, a.operations); err != nil {
		return err
	}
	a.discovered = append(a.discovered, rp.postDecls...)
	return nil
}

func (a *appScanner) parseResponses(file *ast.File) error {
	rp := newResponseParser(a.prog)
	if err := rp.Parse(file, a.responses); err != nil {
		return err
	}
	a.discovered = append(a.discovered, rp.postDecls...)
	return nil
}

func (a *appScanner) parseMeta(file *ast.File) error {
	return newMetaParser(a.input).Parse(file.Doc)
}

// MustExpandPackagePath gets the real package path on disk
func (a *appScanner) MustExpandPackagePath(packagePath string) string {
	pkgRealpath := util.FindInGoSearchPath(packagePath)
	if pkgRealpath == "" {
		log.Fatalf("Can't find package %s \n", packagePath)
	}

	return pkgRealpath
}

type swaggerTypable interface {
	Typed(string, string)
	SetRef(spec.Ref)
}

type selectorParser struct {
	program     *loader.Program
	AddPostDecl func(schemaDecl)
}

func (sp *selectorParser) TypeForSelector(gofile *ast.File, expr *ast.SelectorExpr, prop swaggerTypable) error {
	if pth, ok := expr.X.(*ast.Ident); ok {
		// lookup import
		var selPath string
		for _, imp := range gofile.Imports {
			pv, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				pv = imp.Path.Value
			}
			if imp.Name != nil {
				if imp.Name.Name == pth.Name {
					selPath = pv
					break
				}
			} else {
				parts := strings.Split(pv, "/")
				if len(parts) > 0 && parts[len(parts)-1] == pth.Name {
					selPath = pv
					break
				}
			}
		}
		// find actual struct
		if selPath == "" {
			return fmt.Errorf("no import found for %s", pth.Name)
		}

		pkg := sp.program.Package(selPath)
		if pkg == nil {
			return fmt.Errorf("no package found for %s", selPath)
		}

		// find the file this selector points to
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if gd, ok := decl.(*ast.GenDecl); ok {
					for _, gs := range gd.Specs {
						if ts, ok := gs.(*ast.TypeSpec); ok {
							if ts.Name != nil && ts.Name.Name == expr.Sel.Name {
								// look at doc comments for +swagger:strfmt [name]
								// when found this is the format name, create a schema with that name
								if strfmtName, ok := strfmtName(gd.Doc); ok {
									prop.Typed("string", strfmtName)
									return nil
								}

								switch tpe := ts.Type.(type) {
								case *ast.StructType:
									sd := schemaDecl{file, gd, ts, "", ""}
									sd.inferNames()
									ref, err := spec.NewRef("#/definitions/" + sd.Name)
									if err != nil {
										return err
									}
									prop.SetRef(ref)
									sp.AddPostDecl(sd)
									return nil
								case *ast.Ident:
									for _, fl := range pkg.Files {
										for _, dcl := range fl.Decls {
											if gdd, ok := dcl.(*ast.GenDecl); ok {
												for _, gss := range gdd.Specs {
													if st, ok := gss.(*ast.TypeSpec); ok {
														if st.Name != nil && st.Name.Name == tpe.Name {
															if _, ok := st.Type.(*ast.StructType); ok {
																// At this stage we're no longer interested in actually
																// parsing a struct like this, we're going to reference it instead
																// In addition to referencing, it is added to a bag of discovered schemas
																sd := schemaDecl{fl, gdd, st, "", ""}
																sd.inferNames()
																ref, err := spec.NewRef("#/definitions/" + sd.Name)
																if err != nil {
																	return err
																}
																prop.SetRef(ref)
																sp.AddPostDecl(sd)
																return nil
															}

															// Check if this might be a type decorated with strfmt
															if sfn, ok := strfmtName(gdd.Doc); ok {
																prop.Typed("string", sfn)
																return nil
															}
														}
													}
												}
											}
										}
									}
									return swaggerSchemaForType(tpe.Name, prop)
								case *ast.SelectorExpr:
									return sp.TypeForSelector(file, tpe, prop)
								}

							}
						}
					}
				}
			}
		}

		return fmt.Errorf("schema parser: no string format for %s.%s", pth.Name, expr.Sel.Name)
	}
	return fmt.Errorf("schema parser: no string format for %v", expr.Sel.Name)
}

func swaggerSchemaForType(typeName string, prop swaggerTypable) error {
	switch typeName {
	case "bool":
		prop.Typed("boolean", "")
	case "rune", "string":
		prop.Typed("string", "")
	case "int8":
		prop.Typed("number", "int8")
	case "int16":
		prop.Typed("number", "int16")
	case "int32":
		prop.Typed("number", "int32")
	case "int", "int64":
		prop.Typed("number", "int64")
	case "uint8":
		prop.Typed("number", "uint8")
	case "uint16":
		prop.Typed("number", "uint16")
	case "uint32":
		prop.Typed("number", "uint32")
	case "uint", "uint64":
		prop.Typed("number", "uint64")
	case "float32":
		prop.Typed("number", "float")
	case "float64":
		prop.Typed("number", "double")
	}
	return nil
}

func newMultiLineTagParser(name string, parser valueParser) tagParser {
	return tagParser{
		Name:      name,
		MultiLine: true,
		Parser:    parser,
	}
}

func newSingleLineTagParser(name string, parser valueParser) tagParser {
	return tagParser{
		Name:      name,
		MultiLine: false,
		Parser:    parser,
	}
}

type tagParser struct {
	Name      string
	MultiLine bool
	Lines     []string
	Parser    valueParser
}

func (st *tagParser) Matches(line string) bool {
	return st.Parser.Matches(line)
}

func (st *tagParser) Parse(lines []string) error {
	return st.Parser.Parse(lines)
}

// aggregates lines in header until it sees a tag.
type sectionedParser struct {
	header     []string
	matched    map[string]tagParser
	annotation valueParser

	seenTag        bool
	skipHeader     bool
	setTitle       func([]string)
	setDescription func([]string)
	workedOutTitle bool
	taggers        []tagParser
	currentTagger  *tagParser
	title          []string
	description    []string
}

func (st *sectionedParser) cleanup(lines []string) []string {
	seenLine := -1
	var lastContent int
	var uncommented []string
	for i, v := range lines {
		str := regexp.MustCompile("^[^\\p{L}\\p{N}\\+]*").ReplaceAllString(v, "")
		uncommented = append(uncommented, str)
		if str != "" {
			if seenLine < 0 {
				seenLine = i
			}
			lastContent = i
		}
	}
	return uncommented[seenLine : lastContent+1]
}

func (st *sectionedParser) collectTitleDescription() {
	if st.workedOutTitle {
		return
	}
	if st.setTitle == nil {
		st.header = st.cleanup(st.header)
		return
	}
	hdrs := st.cleanup(st.header)

	st.workedOutTitle = true
	idx := -1
	for i, line := range hdrs {
		if strings.TrimSpace(line) == "" {
			idx = i
			break
		}
	}

	if idx > -1 {

		st.title = hdrs[:idx]
		if len(hdrs) > idx+1 {
			st.header = hdrs[idx+1:]
		} else {
			st.header = nil
		}
		return
	}

	if len(hdrs) > 0 {
		line := hdrs[0]
		if rxPunctuationEnd.MatchString(line) {
			st.title = []string{line}
			st.header = hdrs[1:]
		} else {
			st.header = hdrs
		}
	}
}

func (st *sectionedParser) Title() []string {
	st.collectTitleDescription()
	return st.title
}

func (st *sectionedParser) Description() []string {
	st.collectTitleDescription()
	return st.header
}

func (st *sectionedParser) Parse(doc *ast.CommentGroup) error {
	if doc == nil {
		return nil
	}
COMMENTS:
	for _, c := range doc.List {
		for _, line := range strings.Split(c.Text, "\n") {
			if rxSwaggerAnnotation.MatchString(line) {
				if st.annotation == nil || !st.annotation.Matches(line) {
					break COMMENTS // a new +swagger: annotation terminates this parser
				}

				st.annotation.Parse([]string{line})
				if len(st.header) > 0 {
					st.seenTag = true
				}
				continue
			}

			var matched bool
			for _, tagger := range st.taggers {
				if tagger.Matches(line) {
					st.seenTag = true
					st.currentTagger = &tagger
					matched = true
					break
				}
			}

			if st.currentTagger == nil {
				if !st.skipHeader && !st.seenTag {
					st.header = append(st.header, line)
				}
				// didn't match a tag, moving on
				continue
			}

			if st.currentTagger.MultiLine && matched {
				// the first line of a multiline tagger doesn't count
				continue
			}

			ts, ok := st.matched[st.currentTagger.Name]
			if !ok {
				ts = *st.currentTagger
			}
			ts.Lines = append(ts.Lines, line)
			if st.matched == nil {
				st.matched = make(map[string]tagParser)
			}
			st.matched[st.currentTagger.Name] = ts

			if !st.currentTagger.MultiLine {
				st.currentTagger = nil
			}
		}
	}
	if st.setTitle != nil {
		st.setTitle(st.Title())
	}
	if st.setDescription != nil {
		st.setDescription(st.Description())
	}
	for _, mt := range st.matched {
		if err := mt.Parse(st.cleanup(mt.Lines)); err != nil {
			return err
		}
	}
	return nil
}
