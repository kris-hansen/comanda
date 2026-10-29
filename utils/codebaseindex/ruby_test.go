package codebaseindex

import (
	"reflect"
	"strings"
	"testing"
)

func TestRubyDeclarationsAndScopes(t *testing.T) {
	source := `require 'json'
require_relative("support")
module Shop
  VERSION = '1.0'
  class Cart < Base
    def initialize(items = [])
      @items = items
    end
    def total(currency: "USD")
      @items.each do |item|
        if item.valid?
          item.price
        end
      end
    end
    def empty? = @items.empty?
    def self.build(items) = new(items)
    def name=(value); @name = value; end
    private
    def reset!; @items = []; end
    public
    def size
      0 if @items.nil?
      @items.size
    end
    class << self
      def from_json(data); new(data); end
    end
  end
end
class Other
  def total; 0; end
end
def helper(value = 1); value; end
`
	info := extractRubySymbols("lib/shop/cart.rb", []byte(source))
	var types []string
	for _, typ := range info.Types {
		types = append(types, typ.Name)
	}
	if !reflect.DeepEqual(types, []string{"Shop", "Shop::Cart", "Other"}) {
		t.Fatalf("types: %v", types)
	}
	if !reflect.DeepEqual(info.Imports, []string{"json", "support"}) || !reflect.DeepEqual(info.Constants, []string{"Shop::VERSION"}) {
		t.Fatalf("imports/constants: %+v", info)
	}
	want := []struct {
		name, receiver string
		exported       bool
	}{
		{"initialize", "Shop::Cart", false}, {"total", "Shop::Cart", true},
		{"empty?", "Shop::Cart", true}, {"build", "Shop::Cart.self", true},
		{"name=", "Shop::Cart", true}, {"reset!", "Shop::Cart", false},
		{"size", "Shop::Cart", true}, {"from_json", "Shop::Cart.self", true},
		{"total", "Other", true}, {"helper", "", false},
	}
	if len(info.Functions) != len(want) {
		t.Fatalf("functions: %+v", info.Functions)
	}
	for i, fn := range info.Functions {
		if fn.Name != want[i].name || fn.Receiver != want[i].receiver || fn.IsExported != want[i].exported {
			t.Errorf("function %d: %+v, want %+v", i, fn, want[i])
		}
	}
	if info.Functions[1].Signature != `def total(currency: "USD")` || info.Functions[2].Signature != "def empty?" {
		t.Fatalf("signatures: %+v", info.Functions)
	}
}

func TestRubyIgnoresNonCodeAndDynamicImports(t *testing.T) {
	source := "# class Comment; def fake; end; end\n" +
		"=begin\nclass BlockComment\n=end\n" +
		"text = \"class StringClass; def fake; end; end\"\n" +
		"text = %q{class PercentClass; def fake; end; end}\n" +
		"text = <<~TEXT\nclass HeredocClass\ndef fake\nend\nTEXT\n" +
		"require \"#{dependency}\"\nrequire 'prefix' + suffix\n" +
		"class Real\n  def run(\n    arg = 'end'\n  )\n    arg\n  end\nend\n" +
		"__END__\nclass DataClass; def fake; end; end\n"
	info := extractRubySymbols("test.rb", []byte(source))
	if len(info.Types) != 1 || info.Types[0].Name != "Real" || len(info.Functions) != 1 || info.Functions[0].Receiver != "Real" || len(info.Imports) != 0 {
		t.Fatalf("non-code leaked into symbols: %+v", info)
	}
}

func TestRubyAdapterScan(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"Gemfile":              "source 'https://rubygems.org'\ngem 'rails'\ngem 'pg'\n",
		"Gemfile.lock":         "GEM\n",
		"Rakefile":             "require 'rake'\n",
		"config.ru":            "require_relative 'config/environment'\n",
		"lib/task.rake":        "def task_helper; end\n",
		"lib/user.RB":          "class User\n def save!; end\nend\n",
		"project.gemspec":      "Gem::Specification.new do |spec|\nend\n",
		"vendor/dependency.rb": "class Excluded; end\n",
		"tmp/generated.rb":     "class Excluded; end\n",
		"log/debug.rb":         "class Excluded; end\n",
		"coverage/output.rb":   "class Excluded; end\n",
	} {
		writeStructureFixture(t, root, path, content)
	}
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.OutputFormat = FormatFull
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, languages, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(languages, []string{"ruby"}) || len(scan.GraphFiles) != 7 {
		t.Fatalf("languages %v, files %+v", languages, scan.GraphFiles)
	}
	for _, f := range scan.GraphFiles {
		if f.Path == "Gemfile.lock" {
			if f.Language != "" {
				t.Fatal("lockfile is not Ruby source")
			}
			continue
		}
		if f.Language != "ruby" || f.Symbols == nil {
			t.Errorf("source file not parsed: %+v", f)
		}
	}
	if len(scan.Components) != 1 || scan.Components[0].Language != "ruby" || scan.Components[0].Kind != "backend" {
		t.Fatalf("components: %+v", scan.Components)
	}
	result, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"User", "save!", "rails"} {
		if !strings.Contains(result.Content, text) {
			t.Errorf("index omitted %q", text)
		}
	}
}

func TestRubyDetectionWithoutManifest(t *testing.T) {
	for _, name := range []string{"script.rb", "script.RB", "tasks.rake", "example.gemspec", "config.ru", "Rakefile"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeStructureFixture(t, root, name, "def run; end\n")
			adapters := NewRegistry().Detect(root)
			if len(adapters) != 1 || adapters[0].Name() != "ruby" {
				t.Fatalf("adapters: %v", adapters)
			}
		})
	}
}

func TestRubyKeywordNamesAndControlFlow(t *testing.T) {
	source := `class Service
  attr_accessor :name, "status"
  # Runs the service.
  def run
    value = if ready?
      object.class
    else
      {class: :end, module: :do}
    end
    while ready? do
      work
    end
    for item in items do
      item
    end
  end
  def after; end
  private :after
end
class Final; def call; end; end
`
	info := extractRubySymbols("service.rb", []byte(source))
	if len(info.Types) != 2 || len(info.Functions) != 3 {
		t.Fatalf("keyword names corrupted scopes: %+v", info)
	}
	if !reflect.DeepEqual(info.Types[0].Fields, []string{"name", "status"}) || info.Functions[0].Comments != "Runs the service." {
		t.Fatalf("attributes/comments: %+v", info)
	}
	if info.Functions[1].Receiver != "Service" || info.Functions[1].IsExported || info.Functions[2].Receiver != "Final" {
		t.Fatalf("method scopes/visibility: %+v", info.Functions)
	}
}

func TestRubyLiteralForms(t *testing.T) {
	for _, literal := range []string{
		`/class Fake; def fake; end; end/`,
		`%r{class Fake; def fake; end; end}`,
		`%(class Fake; def fake; end; end)`,
		`%Q{class Fake; def fake; end; end}`,
		"<<'text'\nclass Fake; def fake; end; end\ntext",
	} {
		t.Run(literal, func(t *testing.T) {
			info := extractRubySymbols("literal.rb", []byte("value = "+literal+"\nclass Real; def run; end; end\n"))
			if len(info.Types) != 1 || info.Types[0].Name != "Real" || len(info.Functions) != 1 || info.Functions[0].Receiver != "Real" {
				t.Fatalf("literal leaked symbols: %+v", info)
			}
		})
	}
}

func TestRubyFrameworksAndStaticImports(t *testing.T) {
	info := extractRubySymbols("Gemfile", []byte("gem 'rails'\ngem('pg')\nrequire 'active_record'\nrequire %q(openssl)\nrequire %w(not an import)\nrequire `dynamic`\n"))
	if !reflect.DeepEqual(info.Imports, []string{"rails", "pg", "active_record", "openssl"}) ||
		!reflect.DeepEqual(info.Frameworks, []string{"rails", "activerecord"}) ||
		!reflect.DeepEqual(info.RiskTags, []string{"database", "crypto"}) {
		t.Fatalf("unexpected dependencies/frameworks: %+v", info)
	}
}

func FuzzRubyExtraction(f *testing.F) {
	for _, source := range []string{"", "def", "class << self", "class A; def run(x) = x; end", "x = <<~TEXT\nbody\nTEXT\n", "def x(\n"} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		info := extractRubySymbols("input.rb", []byte(source))
		if info == nil {
			t.Fatal("nil symbols")
		}
	})
}
