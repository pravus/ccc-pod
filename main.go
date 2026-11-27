package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	Args       []string `json:"args" yaml:"args"`
	Dns        []string `json:"dns" yaml:"dns"`
	Entrypoint *string  `json:"entrypoint" yaml:"entrypoint"`
	Env        []string `json:"env" yaml:"env"`
	EnvFile    *string  `json:"envfile" yaml:"envfile"`
	Image      *string  `json:"image" yaml:"image"`
	Hostname   *string  `json:"hostname" yaml:"hostname"`
	Hosts      []string `json:"hosts" yaml:"hosts"`
	Name       *string  `json:"name" yaml:"name"`
	Network    *string  `json:"network" yaml:"network"`
	Publish    []string `json:"publish" yaml:"publish"`
	ReadOnly   *bool    `json:"readonly" yaml:"readonly"`
	Replace    *bool    `json:"replace" yaml:"replace"`
	Rm         *bool    `json:"rm" yaml:"rm"`
	Volumes    []string `json:"volumes" yaml:"volumes"`
	Caps       struct {
		Add  []string `json:"add" yaml:"add"`
		Drop []string `json:"drop" yaml:"drop"`
	} `json:"caps" yaml:"caps"`
}

type Flags struct {
	Bliss       bool
	Detach      bool
	Driver      string
	File        string
	Interactive bool
	Show        bool
	Start       bool
	Stop        bool
	Verbose     bool
}

func main() {
	flags := Flags{}
	flag.BoolVar(&flags.Bliss, `bliss`, false, `who are you?`)
	flag.BoolVar(&flags.Detach, `d`, false, ``)
	flag.BoolVar(&flags.Detach, `detach`, false, `detach from the terminal`)
	flag.StringVar(&flags.Driver, `driver`, ``, `path to driver`)
	flag.StringVar(&flags.File, `f`, ``, `path to the yaml spec file`)
	flag.BoolVar(&flags.Interactive, `i`, false, `enables interactive mode`)
	flag.BoolVar(&flags.Show, `show`, false, `shows the command to be run`)
	flag.BoolVar(&flags.Start, `start`, false, `start the pod`)
	flag.BoolVar(&flags.Stop, `stop`, false, `stop the pod`)
	flag.BoolVar(&flags.Verbose, `v`, false, ``)
	flag.BoolVar(&flags.Verbose, `verbose`, false, `turn on extra logging`)
	flag.Parse()

	if flags.Start && flags.Stop {
		fatal(`ambiguous operations: only provide one of -start or -stop`)
	}

	driver, err := driver(flags.Driver)
	if err != nil {
		fatal(`driver: %s`, err)
	}

	var spec Spec
	if flags.File != `` {
		file, err := os.Open(flags.File)
		if err != nil {
			fatal(`%s`, err)
		}
		defer file.Close()
		if err := yaml.NewDecoder(file).Decode(&spec); err != nil {
			fatal(`yaml: %s`, err)
		}
	}

	if flags.Bliss || flags.Stop {
		if spec.Name == nil || *spec.Name == `` {
			if len(flag.Args()) <= 0 {
				fatal(`-name required`)
			}
			spec.Name = &flag.Args()[0]
		}
	}

	if flags.Bliss {
		err := bliss(flags, driver, spec)
		if err != nil {
			fatal(`bliss: %s`, err.Error())
		}
	}

	cmd := []string{driver}
	switch {
	case flags.Start:
		cmd = append(cmd, `run`)
		if flags.Interactive {
			cmd = append(cmd, []string{`--interactive`, `--tty`}...)
		}
		cmd = addBool(cmd, `detach`, &flags.Detach)
		cmd = addBool(cmd, `rm`, spec.Rm)
		cmd = addBool(cmd, `replace`, spec.Replace)
		cmd = addString(cmd, `name`, spec.Name)
		cmd = addString(cmd, `network`, spec.Network)
		cmd = addString(cmd, `hostname`, spec.Hostname)
		cmd = addSlice(cmd, `add-host`, spec.Hosts)
		cmd = addSlice(cmd, `cap-add`, spec.Caps.Add)
		cmd = addSlice(cmd, `cap-drop`, spec.Caps.Drop)
		cmd = addSlice(cmd, `dns`, spec.Dns)
		cmd = addString(cmd, `env-file`, spec.EnvFile)
		cmd = addSlice(cmd, `env`, spec.Env)
		cmd = addBool(cmd, `read-only`, spec.ReadOnly)
		cmd = addSlice(cmd, `publish`, spec.Publish)
		cmd = addSlice(cmd, `volume`, spec.Volumes)
		cmd = addString(cmd, `entrypoint`, spec.Entrypoint)
		if spec.Image != nil && *spec.Image != `` {
			cmd = append(cmd, *spec.Image)
		}
		cmd = append(cmd, spec.Args...)
		cmd = append(cmd, flag.Args()...)
	case flags.Stop:
		cmd = append(cmd, `stop`)
		cmd = append(cmd, *spec.Name)
	default:
		flag.Usage()
		fatal(`no operation given; provide one of -start or -stop`)
	}

	if flags.Show || flags.Verbose {
		show(cmd)
	}
	if !flags.Show {
		if err := syscall.Exec(driver, cmd, os.Environ()); err != nil {
			fatal(`exec: %s`, err)
		}
	}
}

func driver(given string) (string, error) {
	if given != `` {
		path, err := exec.LookPath(given)
		if err != nil {
			return ``, fmt.Errorf(`path lookup: %w`, err)
		}
		return path, nil
	}
	for _, bin := range []string{`podman`, `docker`} {
		if path, err := exec.LookPath(bin); err == nil {
			return path, nil
		}
	}
	return ``, fmt.Errorf(`driver not found`)
}

func bliss(flags Flags, driver string, spec Spec) error {
	cmd := []string{driver, `rm`, *spec.Name}
	if flags.Verbose {
		show(cmd)
	}
	if flags.Show {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runner := exec.CommandContext(ctx, driver, cmd[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = os.Stdout
	runner.Stderr = os.Stderr

	if err := runner.Run(); err != nil {
		return fmt.Errorf(`run: %w`, err)
	}
	return nil
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

func show(cmd []string) {
	line := ``
	for i, text := range cmd {
		if i != 0 {
			line += ` `
		}
		line += fmt.Sprintf(`%q`, text)
	}
	fmt.Println(line)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
