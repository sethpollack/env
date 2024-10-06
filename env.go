// Package env is a simple, zero-dependencies library to parse environment
// variables into structs.
//
// Example:
//
//	type config struct {
//		Home string `env:"HOME"`
//	}
//	// parse
//	var cfg config
//	err := env.Parse(&cfg)
//	// or parse with generics
//	cfg, err := env.ParseAs[config]()
//
// Check the examples and README for more detailed usage.
package env

import (
	"encoding"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// nolint: gochecknoglobals
var (
	defaultBuiltInParsers = map[reflect.Kind]ParserFunc{
		reflect.Bool: func(v string) (interface{}, error) {
			return strconv.ParseBool(v)
		},
		reflect.String: func(v string) (interface{}, error) {
			return v, nil
		},
		reflect.Int: func(v string) (interface{}, error) {
			i, err := strconv.ParseInt(v, 10, 32)
			return int(i), err
		},
		reflect.Int16: func(v string) (interface{}, error) {
			i, err := strconv.ParseInt(v, 10, 16)
			return int16(i), err
		},
		reflect.Int32: func(v string) (interface{}, error) {
			i, err := strconv.ParseInt(v, 10, 32)
			return int32(i), err
		},
		reflect.Int64: func(v string) (interface{}, error) {
			return strconv.ParseInt(v, 10, 64)
		},
		reflect.Int8: func(v string) (interface{}, error) {
			i, err := strconv.ParseInt(v, 10, 8)
			return int8(i), err
		},
		reflect.Uint: func(v string) (interface{}, error) {
			i, err := strconv.ParseUint(v, 10, 32)
			return uint(i), err
		},
		reflect.Uint16: func(v string) (interface{}, error) {
			i, err := strconv.ParseUint(v, 10, 16)
			return uint16(i), err
		},
		reflect.Uint32: func(v string) (interface{}, error) {
			i, err := strconv.ParseUint(v, 10, 32)
			return uint32(i), err
		},
		reflect.Uint64: func(v string) (interface{}, error) {
			i, err := strconv.ParseUint(v, 10, 64)
			return i, err
		},
		reflect.Uint8: func(v string) (interface{}, error) {
			i, err := strconv.ParseUint(v, 10, 8)
			return uint8(i), err
		},
		reflect.Float64: func(v string) (interface{}, error) {
			return strconv.ParseFloat(v, 64)
		},
		reflect.Float32: func(v string) (interface{}, error) {
			f, err := strconv.ParseFloat(v, 32)
			return float32(f), err
		},
	}
)

func defaultTypeParsers() map[reflect.Type]ParserFunc {
	return map[reflect.Type]ParserFunc{
		reflect.TypeOf(url.URL{}):       parseURL,
		reflect.TypeOf(time.Nanosecond): parseDuration,
		reflect.TypeOf(time.Location{}): parseLocation,
	}
}

func parseURL(v string) (interface{}, error) {
	u, err := url.Parse(v)
	if err != nil {
		return nil, newParseValueError("unable to parse URL", err)
	}
	return *u, nil
}

func parseDuration(v string) (interface{}, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return nil, newParseValueError("unable to parse duration", err)
	}
	return d, err
}

func parseLocation(v string) (interface{}, error) {
	loc, err := time.LoadLocation(v)
	if err != nil {
		return nil, newParseValueError("unable to parse location", err)
	}
	return *loc, nil
}

// ParserFunc defines the signature of a function that can be used within
// `Options`' `FuncMap`.
type ParserFunc func(v string) (interface{}, error)

// OnSetFn is a hook that can be run when a value is set.
type OnSetFn func(tag string, value interface{}, isDefault bool)

// processFieldFn is a function which takes all information about a field and processes it.
type processFieldFn func(refField reflect.Value, refTypeField reflect.StructField, opts Options, fieldParams FieldParams) error

// Options for the parser.
type Options struct {
	// Environment keys and values that will be accessible for the service.
	Environment map[string]string

	// TagName specifies another tag name to use rather than the default 'env'.
	TagName string

	// DefaultValueTagName specifies another default tag name to use rather than the default 'envDefault'.
	DefaultValueTagName string

	// RequiredIfNoDef automatically sets all fields as required if they do not
	// declare 'envDefault'.
	RequiredIfNoDef bool

	// OnSet allows to run a function when a value is set.
	OnSet OnSetFn

	// Prefix define a prefix for every key.
	Prefix string

	// UseFieldNameByDefault defines whether or not `env` should use the field
	// name by default if the `env` key is missing.
	// Note that the field name will be "converted" to conform with environment
	// variable names conventions.
	UseFieldNameByDefault bool

	// Custom parse functions for different types.
	FuncMap map[reflect.Type]ParserFunc

	// Used internally. maps the env variable key to its resolved string value.
	// (for env var expansion)
	rawEnvVars map[string]string
}

func (opts *Options) getRawEnv(s string) string {
	val := opts.rawEnvVars[s]
	if val == "" {
		return opts.Environment[s]
	}
	return val
}

func defaultOptions() Options {
	return Options{
		TagName:             "env",
		DefaultValueTagName: "envDefault",
		Environment:         toMap(os.Environ()),
		FuncMap:             defaultTypeParsers(),
		rawEnvVars:          make(map[string]string),
	}
}

func customOptions(opt Options) Options {
	defOpts := defaultOptions()
	if opt.TagName == "" {
		opt.TagName = defOpts.TagName
	}
	if opt.DefaultValueTagName == "" {
		opt.DefaultValueTagName = defOpts.DefaultValueTagName
	}
	if opt.Environment == nil {
		opt.Environment = defOpts.Environment
	}
	if opt.FuncMap == nil {
		opt.FuncMap = map[reflect.Type]ParserFunc{}
	}
	if opt.rawEnvVars == nil {
		opt.rawEnvVars = defOpts.rawEnvVars
	}
	for k, v := range defOpts.FuncMap {
		if _, exists := opt.FuncMap[k]; !exists {
			opt.FuncMap[k] = v
		}
	}
	return opt
}

func optionsWithMapEnvPrefix(opts Options, mapKey string) Options {
	return Options{
		Environment:           opts.Environment,
		TagName:               opts.TagName,
		DefaultValueTagName:   opts.DefaultValueTagName,
		RequiredIfNoDef:       opts.RequiredIfNoDef,
		OnSet:                 opts.OnSet,
		Prefix:                fmt.Sprintf("%s%s_", opts.Prefix, mapKey),
		UseFieldNameByDefault: opts.UseFieldNameByDefault,
		FuncMap:               opts.FuncMap,
		rawEnvVars:            opts.rawEnvVars,
	}
}

func optionsWithSliceEnvPrefix(opts Options, index int) Options {
	return Options{
		Environment:           opts.Environment,
		TagName:               opts.TagName,
		DefaultValueTagName:   opts.DefaultValueTagName,
		RequiredIfNoDef:       opts.RequiredIfNoDef,
		OnSet:                 opts.OnSet,
		Prefix:                fmt.Sprintf("%s%d_", opts.Prefix, index),
		UseFieldNameByDefault: opts.UseFieldNameByDefault,
		FuncMap:               opts.FuncMap,
		rawEnvVars:            opts.rawEnvVars,
	}
}

func optionsWithEnvPrefix(field reflect.StructField, opts Options) Options {
	return Options{
		Environment:           opts.Environment,
		TagName:               opts.TagName,
		DefaultValueTagName:   opts.DefaultValueTagName,
		RequiredIfNoDef:       opts.RequiredIfNoDef,
		OnSet:                 opts.OnSet,
		Prefix:                opts.Prefix + field.Tag.Get("envPrefix"),
		UseFieldNameByDefault: opts.UseFieldNameByDefault,
		FuncMap:               opts.FuncMap,
		rawEnvVars:            opts.rawEnvVars,
	}
}

// Parse parses a struct containing `env` tags and loads its values from
// environment variables.
func Parse(v interface{}) error {
	return parseInternal(v, setField, defaultOptions())
}

// ParseWithOptions parses a struct containing `env` tags and loads its values from
// environment variables.
func ParseWithOptions(v interface{}, opts Options) error {
	return parseInternal(v, setField, customOptions(opts))
}

// ParseAs parses the given struct type containing `env` tags and loads its
// values from environment variables.
func ParseAs[T any]() (T, error) {
	var t T
	return t, Parse(&t)
}

// ParseWithOptions parses the given struct type containing `env` tags and
// loads its values from environment variables.
func ParseAsWithOptions[T any](opts Options) (T, error) {
	var t T
	return t, ParseWithOptions(&t, opts)
}

// Must panic is if err is not nil, and returns t otherwise.
func Must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

// GetFieldParams parses a struct containing `env` tags and returns information about
// tags it found.
func GetFieldParams(v interface{}) ([]FieldParams, error) {
	return GetFieldParamsWithOptions(v, defaultOptions())
}

// GetFieldParamsWithOptions parses a struct containing `env` tags and returns information about
// tags it found.
func GetFieldParamsWithOptions(v interface{}, opts Options) ([]FieldParams, error) {
	var result []FieldParams
	err := parseInternal(
		v,
		func(_ reflect.Value, _ reflect.StructField, _ Options, fieldParams FieldParams) error {
			if fieldParams.OwnKey != "" {
				result = append(result, fieldParams)
			}
			return nil
		},
		customOptions(opts),
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func parseInternal(v interface{}, processField processFieldFn, opts Options) error {
	ptrRef := reflect.ValueOf(v)
	if ptrRef.Kind() != reflect.Ptr {
		return newAggregateError(NotStructPtrError{})
	}
	ref := ptrRef.Elem()
	if ref.Kind() != reflect.Struct {
		return newAggregateError(NotStructPtrError{})
	}

	return doParse(ref, processField, opts)
}

func doParse(ref reflect.Value, processField processFieldFn, opts Options) error {
	refType := ref.Type()

	var agrErr AggregateError

	for i := 0; i < refType.NumField(); i++ {
		refField := ref.Field(i)
		refTypeField := refType.Field(i)

		if err := doParseField(refField, refTypeField, processField, opts); err != nil {
			if val, ok := err.(AggregateError); ok {
				agrErr.Errors = append(agrErr.Errors, val.Errors...)
			} else {
				agrErr.Errors = append(agrErr.Errors, err)
			}
		}
	}

	if len(agrErr.Errors) == 0 {
		return nil
	}

	return agrErr
}

func doParseField(refField reflect.Value, refTypeField reflect.StructField, processField processFieldFn, opts Options) error {
	if !refField.CanSet() {
		return nil
	}
	if reflect.Ptr == refField.Kind() && !refField.IsNil() {
		return parseInternal(refField.Interface(), processField, optionsWithEnvPrefix(refTypeField, opts))
	}
	if reflect.Struct == refField.Kind() && refField.CanAddr() && refField.Type().Name() == "" {
		return parseInternal(refField.Addr().Interface(), processField, optionsWithEnvPrefix(refTypeField, opts))
	}

	params, err := parseFieldParams(refTypeField, opts)
	if err != nil {
		return err
	}

	if err := processField(refField, refTypeField, opts, params); err != nil {
		return err
	}

	if isStructPtr(refField) && refField.IsNil() && shouldInit(params, optionsWithEnvPrefix(refTypeField, opts)) {
		refField.Set(reflect.New(refField.Type().Elem()))
		refField = refField.Elem()

		if _, ok := opts.FuncMap[refField.Type()]; ok {
			return nil
		}
	}

	if isMapOfStructs(refTypeField, opts) {
		return doParseMap(refField, processField, optionsWithEnvPrefix(refTypeField, opts))
	}

	if reflect.Struct == refField.Kind() {
		return doParse(refField, processField, optionsWithEnvPrefix(refTypeField, opts))
	}

	if isSliceOfStructs(refTypeField, opts) {
		return doParseSlice(refField, processField, optionsWithEnvPrefix(refTypeField, opts))
	}

	return nil
}

func shouldInit(fieldParams FieldParams, opts Options) bool {
	if fieldParams.Init {
		return true
	}

	if fieldParams.InitValue {
		return hasValue(opts)
	}

	return false
}
func isMapOfStructs(refTypeField reflect.StructField, opts Options) bool {
	field := refTypeField.Type
	if reflect.Ptr == field.Kind() {
		field = field.Elem()
	}

	if reflect.Map != field.Kind() {
		return false
	}

	field = field.Elem()

	if reflect.Ptr == field.Kind() {
		field = field.Elem()
	}

	_, ignore := defaultBuiltInParsers[field.Kind()]

	if !ignore {
		_, ignore = opts.FuncMap[field]
	}

	if !ignore {
		_, ignore = reflect.New(field).Interface().(encoding.TextUnmarshaler)
	}

	if !ignore {
		ignore = reflect.Struct != field.Kind()
	}
	return !ignore
}

func isSliceOfStructs(refTypeField reflect.StructField, opts Options) bool {
	field := refTypeField.Type
	if reflect.Ptr == field.Kind() {
		field = field.Elem()
	}

	if reflect.Slice != field.Kind() {
		return false
	}

	field = field.Elem()

	if reflect.Ptr == field.Kind() {
		field = field.Elem()
	}

	_, ignore := defaultBuiltInParsers[field.Kind()]

	if !ignore {
		_, ignore = opts.FuncMap[field]
	}

	if !ignore {
		_, ignore = reflect.New(field).Interface().(encoding.TextUnmarshaler)
	}

	if !ignore {
		ignore = reflect.Struct != field.Kind()
	}
	return !ignore
}

func doParseSlice(ref reflect.Value, processField processFieldFn, opts Options) error {
	if opts.Prefix != "" && !strings.HasSuffix(opts.Prefix, string(underscore)) {
		opts.Prefix += string(underscore)
	}

	var environments []string
	for environment := range opts.Environment {
		if strings.HasPrefix(environment, opts.Prefix) {
			environments = append(environments, environment)
		}
	}

	if len(environments) > 0 {
		counter := 0
		for finished := false; !finished; {
			finished = true
			prefix := fmt.Sprintf("%s%d%c", opts.Prefix, counter, underscore)
			for _, variable := range environments {
				if strings.HasPrefix(variable, prefix) {
					counter++
					finished = false
					break
				}
			}
		}

		sliceType := ref.Type()
		var initialized int
		if reflect.Ptr == ref.Kind() {
			sliceType = sliceType.Elem()
			// Due to the rest of code the pre-initialized slice has no chance for this situation
			initialized = 0
		} else {
			initialized = ref.Len()
		}

		var capacity int
		if capacity = initialized; counter > initialized {
			capacity = counter
		}
		result := reflect.MakeSlice(sliceType, capacity, capacity)
		for i := 0; i < capacity; i++ {
			item := result.Index(i)
			if i < initialized {
				item.Set(ref.Index(i))
			}
			if err := doParse(item, processField, optionsWithSliceEnvPrefix(opts, i)); err != nil {
				return err
			}
		}

		if result.Len() > 0 {
			if reflect.Ptr == ref.Kind() {
				resultPtr := reflect.New(sliceType)
				resultPtr.Elem().Set(result)
				result = resultPtr
			}
			ref.Set(result)
		}
	}

	return nil
}

func doParseMap(refField reflect.Value, processField processFieldFn, opts Options) error {
	mapType := refField.Type()
	elemType := mapType.Elem()

	// Check if the element type has any fields
	if elemType.NumField() == 0 {
		return nil // Can't process an empty struct
	}

	// Create a new map if it's nil
	if refField.IsNil() {
		refField.Set(reflect.MakeMap(mapType))
	}

	prefixLen := len(opts.Prefix)

	mapKeys := make(map[string]bool)
	for envKey := range opts.Environment {
		if !strings.HasPrefix(envKey, opts.Prefix) {
			continue
		}

		trimmedKey := envKey[prefixLen:]
		// Check all fields for a matching env tag
		for i := 0; i < elemType.NumField(); i++ {
			field := elemType.Field(i)
			envTag := field.Tag.Get(opts.TagName)
			if envTag == "" {
				continue
			}

			lastIndex := strings.LastIndex(trimmedKey, "_"+envTag)
			if lastIndex == -1 {
				continue
			}

			mapKey := trimmedKey[:lastIndex]
			mapKeys[mapKey] = true
		}
	}

	// Process each map key
	for mapKey := range mapKeys {
		elem := reflect.New(elemType).Elem()

		// Parse the struct
		if err := doParse(elem, processField, optionsWithMapEnvPrefix(opts, mapKey)); err != nil {
			return err
		}

		// Set the element in the map
		refField.SetMapIndex(reflect.ValueOf(strings.ToLower(mapKey)), elem)
	}

	return nil
}

func setField(refField reflect.Value, refTypeField reflect.StructField, opts Options, fieldParams FieldParams) error {
	value, err := get(fieldParams, opts)
	if err != nil {
		return err
	}

	if value != "" {
		return set(refField, refTypeField, value, opts.FuncMap)
	}

	return nil
}

const underscore rune = '_'

func toEnvName(input string) string {
	var output []rune
	for i, c := range input {
		if c == underscore {
			continue
		}
		if len(output) > 0 && unicode.IsUpper(c) {
			if len(input) > i+1 {
				peek := rune(input[i+1])
				if unicode.IsLower(peek) || unicode.IsLower(rune(input[i-1])) {
					output = append(output, underscore)
				}
			}
		}
		output = append(output, unicode.ToUpper(c))
	}
	return string(output)
}

// FieldParams contains information about parsed field tags.
type FieldParams struct {
	OwnKey          string
	Key             string
	DefaultValue    string
	HasDefaultValue bool
	Required        bool
	LoadFile        bool
	Unset           bool
	NotEmpty        bool
	Expand          bool
	Init            bool
	IgnorePrefix    bool
	InitValue       bool
}

func parseFieldParams(field reflect.StructField, opts Options) (FieldParams, error) {
	ownKey, tags := parseKeyForOption(field.Tag.Get(opts.TagName))
	if ownKey == "" && opts.UseFieldNameByDefault {
		ownKey = toEnvName(field.Name)
	}

	defaultValue, hasDefaultValue := field.Tag.Lookup(opts.DefaultValueTagName)

	result := FieldParams{
		OwnKey:          ownKey,
		Key:             opts.Prefix + ownKey,
		Required:        opts.RequiredIfNoDef,
		DefaultValue:    defaultValue,
		HasDefaultValue: hasDefaultValue,
	}

	for _, tag := range tags {
		switch tag {
		case "":
			continue
		case "file":
			result.LoadFile = true
		case "required":
			result.Required = true
		case "unset":
			result.Unset = true
		case "notEmpty":
			result.NotEmpty = true
		case "expand":
			result.Expand = true
		case "init":
			result.Init = true
		case "ignorePrefix":
			result.IgnorePrefix = true
		case "initValue":
			result.InitValue = true
		default:
			return FieldParams{}, newNoSupportedTagOptionError(tag)
		}
	}

	if result.IgnorePrefix {
		result.Key = ownKey
	}

	return result, nil
}

func get(fieldParams FieldParams, opts Options) (val string, err error) {
	var exists, isDefault bool

	val, exists, isDefault = getOr(fieldParams.Key, fieldParams.DefaultValue, fieldParams.HasDefaultValue, opts.Environment)

	if fieldParams.Expand {
		val = os.Expand(val, opts.getRawEnv)
	}

	opts.rawEnvVars[fieldParams.OwnKey] = val

	if fieldParams.Unset {
		defer os.Unsetenv(fieldParams.Key)
	}

	if fieldParams.Required && !exists && len(fieldParams.OwnKey) > 0 {
		return "", newVarIsNotSetError(fieldParams.Key)
	}

	if fieldParams.NotEmpty && val == "" {
		return "", newEmptyVarError(fieldParams.Key)
	}

	if fieldParams.LoadFile && val != "" {
		filename := val
		val, err = getFromFile(filename)
		if err != nil {
			return "", newLoadFileContentError(filename, fieldParams.Key, err)
		}
	}

	if opts.OnSet != nil {
		if fieldParams.OwnKey != "" {
			opts.OnSet(fieldParams.Key, val, isDefault)
		}
	}
	return val, err
}

// hasValue checks if the struct has any values in the environment variables.
func hasValue(opts Options) bool {
	for key, _ := range opts.Environment {
		if strings.HasPrefix(key, opts.Prefix) {
			return true
		}
	}
	return false
}

// split the env tag's key into the expected key and desired option, if any.
func parseKeyForOption(key string) (string, []string) {
	opts := strings.Split(key, ",")
	return opts[0], opts[1:]
}

func getFromFile(filename string) (value string, err error) {
	b, err := os.ReadFile(filename)
	return string(b), err
}

func getOr(key, defaultValue string, defExists bool, envs map[string]string) (val string, exists bool, isDefault bool) {
	value, exists := envs[key]
	switch {
	case (!exists || key == "") && defExists:
		return defaultValue, true, true
	case exists && value == "" && defExists:
		return defaultValue, true, true
	case !exists:
		return "", false, false
	}

	return value, true, false
}

func set(field reflect.Value, sf reflect.StructField, value string, funcMap map[reflect.Type]ParserFunc) error {
	if tm := asTextUnmarshaler(field); tm != nil {
		if err := tm.UnmarshalText([]byte(value)); err != nil {
			return newParseError(sf, err)
		}
		return nil
	}

	typee := sf.Type
	fieldee := field
	if typee.Kind() == reflect.Ptr {
		typee = typee.Elem()
		fieldee = field.Elem()
	}

	parserFunc, ok := funcMap[typee]
	if ok {
		val, err := parserFunc(value)
		if err != nil {
			return newParseError(sf, err)
		}

		fieldee.Set(reflect.ValueOf(val))
		return nil
	}

	parserFunc, ok = defaultBuiltInParsers[typee.Kind()]
	if ok {
		val, err := parserFunc(value)
		if err != nil {
			return newParseError(sf, err)
		}

		fieldee.Set(reflect.ValueOf(val).Convert(typee))
		return nil
	}

	switch field.Kind() {
	case reflect.Slice:
		return handleSlice(field, value, sf, funcMap)
	case reflect.Map:
		return handleMap(field, value, sf, funcMap)
	}

	return newNoParserError(sf)
}

func handleSlice(field reflect.Value, value string, sf reflect.StructField, funcMap map[reflect.Type]ParserFunc) error {
	separator := sf.Tag.Get("envSeparator")
	if separator == "" {
		separator = ","
	}
	parts := strings.Split(value, separator)

	typee := sf.Type.Elem()
	if typee.Kind() == reflect.Ptr {
		typee = typee.Elem()
	}

	if _, ok := reflect.New(typee).Interface().(encoding.TextUnmarshaler); ok {
		return parseTextUnmarshalers(field, parts, sf)
	}

	parserFunc, ok := funcMap[typee]
	if !ok {
		parserFunc, ok = defaultBuiltInParsers[typee.Kind()]
		if !ok {
			return newNoParserError(sf)
		}
	}

	result := reflect.MakeSlice(sf.Type, 0, len(parts))
	for _, part := range parts {
		r, err := parserFunc(part)
		if err != nil {
			return newParseError(sf, err)
		}
		v := reflect.ValueOf(r).Convert(typee)
		if sf.Type.Elem().Kind() == reflect.Ptr {
			v = reflect.New(typee)
			v.Elem().Set(reflect.ValueOf(r).Convert(typee))
		}
		result = reflect.Append(result, v)
	}
	field.Set(result)
	return nil
}

func handleMap(field reflect.Value, value string, sf reflect.StructField, funcMap map[reflect.Type]ParserFunc) error {
	keyType := sf.Type.Key()
	keyParserFunc, ok := funcMap[keyType]
	if !ok {
		keyParserFunc, ok = defaultBuiltInParsers[keyType.Kind()]
		if !ok {
			return newNoParserError(sf)
		}
	}

	elemType := sf.Type.Elem()
	elemParserFunc, ok := funcMap[elemType]
	if !ok {
		elemParserFunc, ok = defaultBuiltInParsers[elemType.Kind()]
		if !ok {
			return newNoParserError(sf)
		}
	}

	separator := sf.Tag.Get("envSeparator")
	if separator == "" {
		separator = ","
	}

	keyValSeparator := sf.Tag.Get("envKeyValSeparator")
	if keyValSeparator == "" {
		keyValSeparator = ":"
	}

	result := reflect.MakeMap(sf.Type)
	for _, part := range strings.Split(value, separator) {
		pairs := strings.Split(part, keyValSeparator)
		if len(pairs) != 2 {
			return newParseError(sf, fmt.Errorf(`%q should be in "key%svalue" format`, part, keyValSeparator))
		}

		key, err := keyParserFunc(pairs[0])
		if err != nil {
			return newParseError(sf, err)
		}

		elem, err := elemParserFunc(pairs[1])
		if err != nil {
			return newParseError(sf, err)
		}

		result.SetMapIndex(reflect.ValueOf(key).Convert(keyType), reflect.ValueOf(elem).Convert(elemType))
	}

	field.Set(result)
	return nil
}

func asTextUnmarshaler(field reflect.Value) encoding.TextUnmarshaler {
	if reflect.Ptr == field.Kind() {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
	} else if field.CanAddr() {
		field = field.Addr()
	}

	tm, ok := field.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return nil
	}
	return tm
}

func parseTextUnmarshalers(field reflect.Value, data []string, sf reflect.StructField) error {
	s := len(data)
	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(reflect.SliceOf(elemType), s, s)
	for i, v := range data {
		sv := slice.Index(i)
		kind := sv.Kind()
		if kind == reflect.Ptr {
			sv = reflect.New(elemType.Elem())
		} else {
			sv = sv.Addr()
		}
		tm := sv.Interface().(encoding.TextUnmarshaler)
		if err := tm.UnmarshalText([]byte(v)); err != nil {
			return newParseError(sf, err)
		}
		if kind == reflect.Ptr {
			slice.Index(i).Set(sv)
		}
	}

	field.Set(slice)

	return nil
}

// ToMap Converts list of env vars as provided by os.Environ() to map you
// can use as Options.Environment field
func ToMap(env []string) map[string]string {
	return toMap(env)
}

func isStructPtr(v reflect.Value) bool {
	return reflect.Ptr == v.Kind() && v.Type().Elem().Kind() == reflect.Struct
}
