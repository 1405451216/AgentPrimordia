// Package config 提供统一配置加载框架。
//
// 加载优先级（低 → 高）：YAML 文件 < 环境变量 < 命令行 flags
//
// 使用示例：
//
//	type MyConfig struct {
//	    ServerHost string `yaml:"server_host" env:"SERVER_HOST" flag:"host"`
//	    ServerPort int    `yaml:"server_port" env:"SERVER_PORT" flag:"port"`
//	}
//
//	cfg := &MyConfig{}
//	err := config.New(cfg, "AP").
//	    LoadYAML(".ap.yaml").
//	    LoadEnv().
//	    LoadFlags().
//	    Validate()
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader 统一配置加载器。
type Loader struct {
	cfg       any
	envPrefix string
	// validators 在 Validate 时按序执行
	validators []func() error
	// flagSet 用于自定义 flag 集（可选）
	flagSet *flag.FlagSet
	// loaded 标记是否已调用过 LoadYAML
	loaded bool
}

// New 创建配置加载器。
// cfg 必须是非 nil 指针，envPrefix 是环境变量前缀（如 "AP"）。
func New(cfg any, envPrefix string) (*Loader, error) {
	if cfg == nil {
		return nil, errors.New("config: cfg must not be nil")
	}
	v := reflect.ValueOf(cfg)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, errors.New("config: cfg must be a non-nil pointer")
	}
	return &Loader{
		cfg:       cfg,
		envPrefix: envPrefix,
		flagSet:   flag.CommandLine,
	}, nil
}

// LoadYAML 从 YAML 文件加载配置。
func (l *Loader) LoadYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在时跳过（允许纯 ENV/flags 配置）
			return nil
		}
		return fmt.Errorf("config: read YAML %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, l.cfg); err != nil {
		return fmt.Errorf("config: parse YAML %q: %w", path, err)
	}
	l.loaded = true
	return nil
}

// LoadEnv 从环境变量加载配置，覆盖 YAML 中的值。
// 环境变量命名规则：{envPrefix}_{TAG}，如 AP_SERVER_HOST。
func (l *Loader) LoadEnv() error {
	return l.loadEnvReflect(reflect.ValueOf(l.cfg).Elem(), l.envPrefix)
}

func (l *Loader) loadEnvReflect(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		// 处理嵌入结构体
		if field.Anonymous && fv.Kind() == reflect.Struct {
			if err := l.loadEnvReflect(fv, prefix); err != nil {
				return err
			}
			continue
		}

		tag := field.Tag.Get("env")
		if tag == "" || tag == "-" {
			continue
		}

		envKey := prefix + "_" + tag
		val, ok := os.LookupEnv(envKey)
		if !ok {
			continue
		}

		if err := setField(fv, val); err != nil {
			return fmt.Errorf("config: env %s=%q: %w", envKey, val, err)
		}
	}
	return nil
}

// LoadFlags 从命令行 flags 加载配置，覆盖 YAML 和环境变量。
func (l *Loader) LoadFlags() error {
	// 收集已定义的 flags（避免重复定义）
	defined := make(map[string]bool)
	l.flagSet.Visit(func(f *flag.Flag) {
		defined[f.Name] = true
	})

	t := reflect.ValueOf(l.cfg).Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous {
			continue
		}
		tag := field.Tag.Get("flag")
		if tag == "" || tag == "-" {
			continue
		}
		if defined[tag] {
			continue
		}
		// 注册 flag 并绑定到字段
		fv := reflect.ValueOf(l.cfg).Elem().Field(i)
		registerFlag(l.flagSet, tag, fv, field.Tag.Get("usage"))
	}

	// 解析 flags
	if !l.flagSet.Parsed() {
		if err := l.flagSet.Parse(os.Args[1:]); err != nil {
			return fmt.Errorf("config: parse flags: %w", err)
		}
	}

	// 将已设置的 flag 值写回结构体
	l.flagSet.Visit(func(f *flag.Flag) {
		setByFlag(reflect.ValueOf(l.cfg).Elem(), f.Name, f.Value.String())
	})

	return nil
}

// Validate 执行所有注册的校验函数。
func (l *Loader) Validate() error {
	for _, v := range l.validators {
		if err := v(); err != nil {
			return err
		}
	}
	return nil
}

// AddValidator 添加自定义校验函数。
func (l *Loader) AddValidator(fn func() error) *Loader {
	l.validators = append(l.validators, fn)
	return l
}

// MustLoad 是 LoadYAML + LoadEnv + LoadFlags + Validate 的便捷方法。
// 失败时返回错误。
func (l *Loader) MustLoad(path string) error {
	if err := l.LoadYAML(path); err != nil {
		return err
	}
	if err := l.LoadEnv(); err != nil {
		return err
	}
	if err := l.LoadFlags(); err != nil {
		return err
	}
	return l.Validate()
}

// Config 返回加载后的配置。
func (l *Loader) Config() any {
	return l.cfg
}

// ===== 内部辅助 =====

func setField(fv reflect.Value, val string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer %q: %w", val, err)
		}
		fv.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer %q: %w", val, err)
		}
		fv.SetUint(u)
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid boolean %q: %w", val, err)
		}
		fv.SetBool(b)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("invalid float %q: %w", val, err)
		}
		fv.SetFloat(f)
	case reflect.Slice:
		// 简单逗号分隔的字符串切片
		parts := strings.Split(val, ",")
		s := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
		for i, p := range parts {
			s.Index(i).SetString(strings.TrimSpace(p))
		}
		fv.Set(s)
	default:
		return fmt.Errorf("unsupported field kind %v", fv.Kind())
	}
	return nil
}

func registerFlag(fs *flag.FlagSet, name string, fv reflect.Value, usage string) {
	switch fv.Kind() {
	case reflect.String:
		fs.String(name, fv.String(), usage)
	case reflect.Int, reflect.Int64:
		fs.Int(name, int(fv.Int()), usage)
	case reflect.Uint, reflect.Uint64:
		fs.Uint(name, uint(fv.Uint()), usage)
	case reflect.Bool:
		fs.Bool(name, fv.Bool(), usage)
	case reflect.Float64:
		fs.Float64(name, fv.Float(), usage)
	}
}

func setByFlag(v reflect.Value, name, val string) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)
		if field.Anonymous && fv.Kind() == reflect.Struct {
			setByFlag(fv, name, val)
			continue
		}
		if field.Tag.Get("flag") == name {
			// 类型转换失败时保留字段默认值（错误由调用方在上游已做校验）
			_ = setField(fv, val)
			return
		}
	}
}

// ToJSON 将配置序列化为 JSON 字符串（用于调试/日志）。
func ToJSON(cfg any) string {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(data)
}
