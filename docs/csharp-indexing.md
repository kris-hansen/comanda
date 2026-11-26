# C# indexing

C# projects are automatically detected by `.sln`, `.slnx`, or `.csproj` files,
`global.json`, `Directory.Build.props`, `Directory.Build.targets`, or `.cs`
source extensions (including mixed-case extensions). Project and solution files
are retained as configuration without being parsed as C#.

```sh
comanda index capture ./my-csharp-project -n my-csharp-project --format full --graph
comanda graph build my-csharp-project
```

The adapter extracts file-scoped and block namespaces, classes, structs,
interfaces, enums, records, delegates, nested types, methods, constructors,
fields, properties, constants, and adjacent documentation comments. Type and
method receivers use namespace-qualified names. Interface members are public by
default; other member visibility follows the declared `public`/`private`
modifiers. Enum members and `const` fields become qualified constants.

`using` directives become dependency imports, including `global using`, `using
static`, and aliases (the alias target is recorded). Imports identify ASP.NET
Core, Entity Framework Core, gRPC, xUnit, NUnit, and MSTest, with database,
authentication, cryptography, networking, filesystem, concurrency, subprocess,
and reflection risk tags for recognized dependencies. ASP.NET Core and gRPC
components are classified as backends. Solution and project files mark C#
component boundaries, including nested projects.

The extractor runs in Go and never requires .NET or executes project code. It
skips line and block comments, preprocessor directive lines, regular and
verbatim strings, interpolated and raw strings, and character literals. `bin`,
`obj`, `.vs`, `TestResults`, and `packages` directories are excluded, along with
common generated C# file suffixes.

This is a lexical indexer, rather than a complete C# grammar or compiler.
Conditional compilation branches are not evaluated, source generators and
implicit global usings are not run, overload resolution is not performed, and
complex generic aliases or unusual declaration forms may produce incomplete
extraction. Methods in partial types declared across different files may not
receive an inferred ownership edge. Use a parser plugin when an application
needs a richer semantic model.
