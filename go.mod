module github.com/Harshmaury/Atlas

go 1.25.0

require (
	github.com/mattn/go-sqlite3 v1.14.34
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/Harshmaury/Canon v1.0.0

replace github.com/Harshmaury/Nexus => ../nexus

replace github.com/Harshmaury/Canon => ../canon
