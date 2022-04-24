# gget - git-dumper sandbox
Creates a Docker ***Python*** image that installs **[git-dumper]("https://github.com/arthaud/git-dumper")** and then runs a container with git-dumper **-u url** **-o outputDirectory**

## prerequisites

* Docker Deamon CLI

## How to use 
```bash
$ go run main.go -u http://example.com/.git -o output/dir
```
OR build the program