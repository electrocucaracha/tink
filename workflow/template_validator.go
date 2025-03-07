package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/docker/distribution/reference"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	errEmptyName              = "name cannot be empty"
	errInvalidLength          = "name cannot have more than 200 characters: %s"
	errTemplateInvalidVersion = "invalid template version: %s"
	errTaskDuplicateName      = "two tasks in a template cannot have same name: %s"
	errActionDuplicateName    = "two actions in a task cannot have same name: %s"
	errActionInvalidImage     = "invalid action image: %s"
	errTemplateParsing        = "failed to parse template with ID %s"
	errInvalidHardwareAddress = "failed to render template, invalid hardware address: %v"
)

// Parse parses the template yaml content into a Workflow.
func Parse(yamlContent []byte) (*Workflow, error) {
	var workflow Workflow

	err := yaml.UnmarshalStrict(yamlContent, &workflow)
	if err != nil {
		return &Workflow{}, errors.Wrap(err, "parsing yaml data")
	}

	if err = validate(&workflow); err != nil {
		return &Workflow{}, errors.Wrap(err, "validating workflow template")
	}

	return &workflow, nil
}

// MustParse parse a slice of bytes to a template. It an error occurs the
// function triggers a panic. Common utility for testing purpose.
func MustParse(yamlContent []byte) *Workflow {
	w, err := Parse(yamlContent)
	if err != nil {
		panic(err)
	}
	return w
}

// MustParseFromFile parse a template from a file and it panics if any error is
// detected. Ideal to be used in testing.
func MustParseFromFile(path string) *Workflow {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		panic(err)
	}
	return MustParse(content)
}

// RenderTemplate renders the workflow template with regard to the given hardware details.
func RenderTemplate(templateID, templateData string, devices []byte) (string, error) {
	var hardware map[string]interface{}
	err := json.Unmarshal(devices, &hardware)
	if err != nil {
		err = errors.Wrapf(err, errTemplateParsing, templateID)
		return "", err
	}

	_, buf, err := RenderTemplateHardware(templateID, templateData, hardware)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderTemplateHardware renders the workflow template and returns the Workflow and the interpolated bytes.
func RenderTemplateHardware(templateID, templateData string, hardware map[string]interface{}) (*Workflow, *bytes.Buffer, error) {
	t := template.New("workflow-template").
		Option("missingkey=error").
		Funcs(templateFuncs)
	_, err := t.Parse(templateData)
	if err != nil {
		err = errors.Wrapf(err, errTemplateParsing, templateID)
		return nil, nil, err
	}

	buf := new(bytes.Buffer)
	if err = t.Execute(buf, hardware); err != nil {
		err = errors.Wrapf(err, errTemplateParsing, templateID)
		return nil, nil, err
	}

	wf, err := Parse(buf.Bytes())
	if err != nil {
		return nil, nil, err
	}
	for _, task := range wf.Tasks {
		if task.WorkerAddr == "" {
			return nil, nil, fmt.Errorf(errInvalidHardwareAddress, hardware)
		}
	}
	return wf, buf, nil
}

// validate validates a workflow template against certain requirements.
func validate(wf *Workflow) error {
	if hasEmptyName(wf.Name) {
		return errors.New(errEmptyName)
	}
	if !hasValidLength(wf.Name) {
		return errors.Errorf(errInvalidLength, wf.Name)
	}

	if wf.Version != "0.1" {
		return errors.Errorf(errTemplateInvalidVersion, wf.Version)
	}

	if len(wf.Tasks) == 0 {
		return errors.New("template must have at least one task defined")
	}

	taskNameMap := make(map[string]struct{})
	for _, task := range wf.Tasks {
		if hasEmptyName(task.Name) {
			return errors.New(errEmptyName)
		}
		if !hasValidLength(task.Name) {
			return errors.Errorf(errInvalidLength, task.Name)
		}

		_, ok := taskNameMap[task.Name]
		if ok {
			return errors.Errorf(errTaskDuplicateName, task.Name)
		}

		taskNameMap[task.Name] = struct{}{}
		actionNameMap := make(map[string]struct{})
		for _, action := range task.Actions {
			if hasEmptyName(action.Name) {
				return errors.New(errEmptyName)
			}

			if !hasValidLength(action.Name) {
				return errors.Errorf(errInvalidLength, action.Name)
			}

			if !hasValidImageName(action.Image) {
				return errors.Errorf(errActionInvalidImage, action.Image)
			}

			_, ok := actionNameMap[action.Name]
			if ok {
				return errors.Errorf(errActionDuplicateName, action.Name)
			}
			actionNameMap[action.Name] = struct{}{}
		}
	}
	return nil
}

func hasEmptyName(name string) bool {
	return name == ""
}

func hasValidLength(name string) bool {
	return len(name) < 200
}

func hasValidImageName(name string) bool {
	_, err := reference.ParseNormalizedNamed(name)
	return err == nil
}
