# Ruby indexing

Ruby projects are automatically detected by `Gemfile`, `Rakefile`,
`.ruby-version`, `config.ru`, or Ruby source extensions. The built-in adapter
supports `.rb` (including mixed-case extensions), `.rake`, `.gemspec`, `.ru`,
and the extensionless `Gemfile` and `Rakefile` DSLs. `Gemfile.lock` is retained
as configuration, without parsing it as Ruby.

```sh
comanda index capture ./my-ruby-project -n my-ruby-project --format full --graph
comanda graph build my-ruby-project
```

The adapter extracts classes, modules, qualified nested names, constants,
method signatures and adjacent comments. It distinguishes instance methods
from class methods (including `class << self`), handles endless methods, and
records `attr_reader`, `attr_writer`, and `attr_accessor` names as type fields.
Visibility sections and named `private`/`protected`/`public` declarations are
reflected in method export flags; top-level methods and `initialize` are private.
Class and instance methods with the same name remain separate graph nodes.

Static string arguments to `require`, `require_relative`, `load`, and `gem`
become dependency imports. Dynamic arguments are omitted. Imports can identify
Rails, Sinatra, Roda, Hanami, Active Record, RSpec, Minitest, and Sidekiq, with
database, authentication, and cryptography tags for recognized dependencies.
`Gemfile` marks Ruby component boundaries, including nested applications;
recognized web frameworks classify those components as backends.

The extractor runs in Go and never executes Ruby or project code. It skips
comments, quoted and percent literals, common regular-expression literals,
heredocs with uppercase or quoted delimiters, and the `__END__` data section.
Ruby namespaces are preserved in type names; package grouping uses the source
file's directory. `vendor`, `.bundle`, `coverage`, `tmp`, and `log` directories
are excluded.

This is a lexical indexer, rather than a complete Ruby grammar or runtime
resolver. Metaprogrammed declarations (`define_method`, `Class.new`), dynamically
computed imports, runtime mixins/autoloads, and full Ruby visibility semantics
are not resolved. Import strings are preserved; `require_relative` does not
currently resolve to a local file edge. Interpolation and unusual literal or
statement forms may produce incomplete extraction. Use a parser plugin when
an application needs a richer semantic model.
