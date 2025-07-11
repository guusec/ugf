package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

type pattern struct {
	Flags    []string `json:"flags,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	Engine   string   `json:"engine,omitempty"`
}

// Allow backward-compatible unmarshalling for flags as string or []string
func (p *pattern) UnmarshalJSON(data []byte) error {
	type Alias pattern
	aux := &struct {
		Flags interface{} `json:"flags,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	switch v := aux.Flags.(type) {
	case string:
		if v != "" {
			p.Flags = strings.Fields(v)
		}
	case []interface{}:
		flags := []string{}
		for _, f := range v {
			if s, ok := f.(string); ok {
				flags = append(flags, s)
			}
		}
		p.Flags = flags
	}
	return nil
}

func main() {
	var saveMode bool
	flag.BoolVar(&saveMode, "save", false, "save a pattern (e.g: ugf -save pat-name -Hnri 'search-pattern')")

	var listMode bool
	flag.BoolVar(&listMode, "list", false, "list available patterns")

	var dumpMode bool
	flag.BoolVar(&dumpMode, "dump", false, "prints the grep command rather than executing it")

	flag.Parse()

	if listMode {
		pats, err := getPatterns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return
		}
		fmt.Println(strings.Join(pats, "\n"))
		return
	}

	if saveMode {
		name := flag.Arg(0)
		flags := flag.Arg(1)
		pattern := flag.Arg(2)

		err := savePattern(name, flags, pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		return
	}

	patName := flag.Arg(0)
	files := flag.Arg(1)
	if files == "" {
		files = "."
	}

	patDir, err := getPatternDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "unable to open user's pattern directory")
		return
	}

	filename := filepath.Join(patDir, patName+".json")
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, "no such pattern")
		return
	}
	defer f.Close()

	pat := pattern{}
	dec := json.NewDecoder(f)
	err = dec.Decode(&pat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pattern file '%s' is malformed: %s\n", filename, err)
		return
	}

	if pat.Pattern == "" {
		// check for multiple patterns
		if len(pat.Patterns) == 0 {
			fmt.Fprintf(os.Stderr, "pattern file '%s' contains no pattern(s)\n", filename)
			return
		}
		pat.Pattern = "(" + strings.Join(pat.Patterns, "|") + ")"
	}

	operator := pat.Engine
	if operator == "" {
		operator = "ugrep"
	}

	if dumpMode {
		if operator == "ugrep" && len(pat.Patterns) > 0 {
			args := []string{}
			args = append(args, pat.Flags...)
			for _, p := range pat.Patterns {
				args = append(args, "-e", p)
			}
			if files != "" {
				args = append(args, files)
			}
			fmt.Printf("%s %s\n", operator, strings.Join(args, " "))
		} else {
			fmt.Printf("%s %s %q %v\n",
				operator, strings.Join(pat.Flags, " "), pat.Pattern, files)
		}
	} else {
		var cmd *exec.Cmd
		args := []string{}
		if operator == "ugrep" && len(pat.Patterns) > 0 {
			args = append(args, pat.Flags...)
			for _, p := range pat.Patterns {
				args = append(args, "-e", p)
			}
			if !stdinIsPipe() && files != "" {
				args = append(args, files)
			}
		} else {
			args = append(args, pat.Flags...)
			args = append(args, pat.Pattern)
			if !stdinIsPipe() && files != "" {
				args = append(args, files)
			}
		}
		cmd = exec.Command(operator, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

func getPatternDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	path := filepath.Join(usr.HomeDir, ".config/gf")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// .config/gf exists
		return path, nil
	}
	return filepath.Join(usr.HomeDir, ".gf"), nil
}

func savePattern(name, flags, pat string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}
	if pat == "" {
		return errors.New("pattern cannot be empty")
	}
	p := &pattern{
		Flags:   strings.Fields(flags),
		Pattern: pat,
	}

	patDir, err := getPatternDir()
	if err != nil {
		return fmt.Errorf("failed to determine pattern directory: %s", err)
	}

	path := filepath.Join(patDir, name+".json")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return fmt.Errorf("failed to create pattern file: %s", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")

	err = enc.Encode(p)
	if err != nil {
		return fmt.Errorf("failed to write pattern file: %s", err)
	}

	return nil
}

func getPatterns() ([]string, error) {
	out := []string{}

	patDir, err := getPatternDir()
	if err != nil {
		return out, fmt.Errorf("failed to determine pattern directory: %s", err)
	}

	files, err := filepath.Glob(patDir + "/*.json")
	if err != nil {
		return out, err
	}

	for _, f := range files {
		f = f[len(patDir)+1 : len(f)-5]
		out = append(out, f)
	}

	return out, nil
}

func stdinIsPipe() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) == 0
}
