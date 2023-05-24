package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	CapAdds  []string `json:"cap-adds"`
	Dns      []string `json:"dns"`
	Env      []string `json:"env"`
	EnvFile  *string  `json:"env-file"`
	Image    *string  `json:"image"`
	Hostname *string  `json:"hostname"`
	Name     *string  `json:"name"`
	Network  *string  `json:"network"`
	Publish  []string `json:"publish"`
	Replace  *bool    `json:"replace"`
	Rm       *bool    `json:"rm"`
	Volumes  []string `json:"volumes"`
}

type Flags struct {
	D     *string
	F     *string
	I     *bool
	N     *string
	Show  *bool
	Start *bool
	Stop  *bool
}

func main() {
	flags := Flags{
		D:     flag.String(`d`, ``, `path to driver`),
		F:     flag.String(`f`, ``, `path to the yaml arg file`),
		I:     flag.Bool(`i`, false, `enables interactive mode`),
		Show:  flag.Bool(`show`, false, `shows the command to be run`),
		Start: flag.Bool(`start`, false, `start the pod`),
		Stop:  flag.Bool(`stop`, false, `stop the pod`),
	}
	flag.Parse()

	driver, err := driver(*flags.D)
	if err != nil {
		fatal(`driver: %s`, err)
	}

	var spec Spec
	if *flags.F != `` {
		file, err := os.Open(*flags.F)
		if err != nil {
			fatal(`%s`, err)
		}
		defer file.Close()
		if err := yaml.NewDecoder(file).Decode(&spec); err != nil {
			fatal(`yaml: %s`, err)
		}
	}

	cmd := []string{driver}
	switch {
	case *flags.Start:
		cmd = append(cmd, `run`)
		if *flags.I {
			cmd = append(cmd, `-it`)
		}
		cmd = addBool(cmd, `rm`, spec.Rm)
		cmd = addBool(cmd, `replace`, spec.Replace)
		cmd = addString(cmd, `name`, spec.Name)
		cmd = addString(cmd, `network`, spec.Network)
		cmd = addString(cmd, `hostname`, spec.Hostname)
		cmd = addSlice(cmd, `cap-add`, spec.CapAdds)
		cmd = addSlice(cmd, `dns`, spec.Dns)
		cmd = addString(cmd, `env-file`, spec.EnvFile)
		cmd = addSlice(cmd, `env`, spec.Env)
		cmd = addSlice(cmd, `publish`, spec.Publish)
		cmd = addSlice(cmd, `volume`, spec.Volumes)
		if spec.Image != nil && *spec.Image != `` {
			cmd = append(cmd, *spec.Image)
		}
		cmd = append(cmd, flag.Args()...)
	case *flags.Stop:
		cmd = append(cmd, `stop`)
		if spec.Name == nil || *spec.Name == `` {
			if len(flag.Args()) <= 0 {
				fatal(`name required`)
			}
			spec.Name = &flag.Args()[0]
		}
		cmd = append(cmd, *spec.Name)
	}

	if *flags.Show {
		fmt.Println(strings.Join(cmd, ` `))
	} else {
		if err := syscall.Exec(driver, cmd, os.Environ()); err != nil {
			fatal(`exec: %s`, err)
		}
	}
}

func driver(given string) (string, error) {
	if given != `` {
		return given, nil
	}
	var paths []string
	var env = os.Getenv(`PATH`)
	if index := strings.Index(env, `:`); index < 0 {
		paths = []string{env}
	} else {
		paths = strings.Split(env, `:`)
	}
	for _, path := range paths {
		if path[len(path)-1] != '/' {
			path += `/`
		}
		for _, bin := range []string{`podman`, `docker`} {
			target := path + bin
			info, err := os.Stat(target)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return ``, err
			}
			if info.IsDir() {
				continue
			}
			// FIXME: check if we can execute it
			return target, nil
		}
	}
	return ``, fmt.Errorf(`driver not found`)
}

func addBool(target []string, option string, condition *bool) []string {
	if condition != nil && *condition {
		target = append(target, `--`+option)
	}
	return target
}

func addSlice(target []string, option string, source []string) []string {
	for _, value := range source {
		target = append(target, `--`+option+`=`+value)
	}
	return target
}

func addString(target []string, option string, value *string) []string {
	if value != nil && *value != `` {
		target = append(target, `--`+option+`=`+*value)
	}
	return target
}

func fatal(format string, args ...any) {
	fmt.Println(fmt.Sprintf(format, args...))
	os.Exit(1)
}
