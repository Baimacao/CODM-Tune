package sys

import (
	"bytes"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Scheme is one power plan.
type Scheme struct {
	GUID   string
	Name   string
	Active bool
}

const UltimateTemplate = "e9a42b02-d5df-448d-aa00-03f14749eb61"

var guidRE = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// DecodeConsole turns the GBK console output of Windows tools into UTF-8.
func DecodeConsole(b []byte) string { return decodeConsole(b) }

// decodeConsole turns the GBK console output of powercfg into UTF-8.
func decodeConsole(b []byte) string {
	r := transform.NewReader(bytes.NewReader(b), simplifiedchinese.GBK.NewDecoder())
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return string(b)
	}
	return buf.String()
}

func runPowercfg(args ...string) (string, error) {
	cmd := exec.Command("powercfg", args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return decodeConsole(out), err
}

// ActiveScheme reports the active plan.
func ActiveScheme() (Scheme, error) {
	out, err := runPowercfg("-getactivescheme")
	if err != nil {
		return Scheme{}, err
	}
	g := guidRE.FindString(out)
	if g == "" {
		return Scheme{}, errors.New("powercfg: no GUID in output")
	}
	return Scheme{GUID: strings.ToLower(g), Name: schemeName(out), Active: true}, nil
}

// ListSchemes parses `powercfg -list`.
func ListSchemes() ([]Scheme, error) {
	out, err := runPowercfg("-list")
	if err != nil {
		return nil, err
	}
	var out2 []Scheme
	for _, line := range strings.Split(out, "\n") {
		g := guidRE.FindString(line)
		if g == "" {
			continue
		}
		out2 = append(out2, Scheme{
			GUID:   strings.ToLower(g),
			Name:   schemeName(line),
			Active: strings.Contains(line, "*"),
		})
	}
	return out2, nil
}

func schemeName(s string) string {
	l := strings.Index(s, "(")
	r := strings.LastIndex(s, ")")
	if l >= 0 && r > l {
		return strings.TrimSpace(s[l+1 : r])
	}
	return ""
}

// FindUltimate looks for an already installed Ultimate Performance plan.
func FindUltimate() (Scheme, bool) {
	list, err := ListSchemes()
	if err != nil {
		return Scheme{}, false
	}
	for _, s := range list {
		n := strings.ToLower(s.Name)
		if strings.Contains(n, "卓越性能") || strings.Contains(n, "ultimate performance") {
			return s, true
		}
	}
	return Scheme{}, false
}

// InjectUltimate returns an Ultimate Performance plan, duplicating the
// built-in template when the plan is not installed yet. It then activates it.
func InjectUltimate() (scheme Scheme, created bool, err error) {
	s, ok := FindUltimate()
	if !ok {
		out, derr := runPowercfg("-duplicatescheme", UltimateTemplate)
		if derr != nil {
			return Scheme{}, false, errors.New("powercfg -duplicatescheme: " + strings.TrimSpace(out))
		}
		g := guidRE.FindString(out)
		if g == "" {
			return Scheme{}, false, errors.New("powercfg -duplicatescheme returned no GUID")
		}
		s = Scheme{GUID: strings.ToLower(g), Name: schemeName(out)}
		created = true
	}
	if err := SetActive(s.GUID); err != nil {
		return s, created, err
	}
	s.Active = true
	return s, created, nil
}

// SetActive switches the active power plan.
func SetActive(guid string) error {
	out, err := runPowercfg("-setactive", guid)
	if err != nil {
		return errors.New("powercfg -setactive: " + strings.TrimSpace(out))
	}
	return nil
}
