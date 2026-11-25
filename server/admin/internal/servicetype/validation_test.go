package servicetype

import (
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestServiceTypeValidation(t *testing.T) {
	if err := Validate(model.ServiceType{Name: "game"}); err != nil {
		t.Fatalf("expected valid global type, got %v", err)
	}
	if err := Validate(model.ServiceType{Name: "Game"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
}

func TestFixedPreset(t *testing.T) {
	fields := []model.ServiceParameterField{{Flag: "--mode", Label: "模式", Kind: "fixed", DefaultValue: "prod"}}
	for _, stored := range [][]model.ServiceParam{nil, {{Flag: "--mode", Value: "old"}}, {{Flag: "--mode", Value: "prod"}}} {
		got, err := MergeParameterValues(fields, stored, nil)
		if err != nil || len(got) != 1 || got[0].Value != "prod" {
			t.Fatalf("merged %v: %v, %v", stored, got, err)
		}
	}
	if _, err := MergeParameterValues(fields, nil, map[string]string{"--mode": "old"}); err == nil {
		t.Fatal("accepted instance override")
	}
	if err := ValidateFixedArgs(fields, "--mode=old"); err == nil {
		t.Fatal("accepted extra argument override")
	}
	fields[0].DefaultValue = " "
	if err := validateParameterFields(fields); err == nil {
		t.Fatal("accepted blank preset")
	}
}
