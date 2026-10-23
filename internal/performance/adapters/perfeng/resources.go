package perfeng

import (
	"errors"
	"reflect"
)

type resourceKey struct{ kind, issuer, id string }

func key(kind string, reference any) resourceKey {
	value := object(reference)
	return resourceKey{kind, value["issuer"].(string), value["id"].(string)}
}

func checkResources(items []any) error {
	resources := make(map[resourceKey]map[string]any, len(items))
	for _, item := range items {
		value := object(item)
		identity := key(value["kind"].(string), value["reference"])
		if resources[identity] != nil {
			return errors.New("duplicate catalog resource identity")
		}
		resources[identity] = value
	}
	resolve := func(kind string, ref any) (map[string]any, error) {
		resource := resources[key(kind, ref)]
		if resource == nil || !reflect.DeepEqual(resource["reference"], ref) {
			return nil, errors.New("missing or substituted catalog resource")
		}

		return resource, nil
	}
	for _, item := range items {
		if err := checkResource(object(item), resolve); err != nil {
			return err
		}
	}

	return nil
}

type resolver func(string, any) (map[string]any, error)

func checkResource(item map[string]any, resolve resolver) error {
	switch item["kind"] {
	case "workload":
		return checkWorkload(item, resolve)
	case "profile":
		return checkProfile(item, resolve)
	case "baseline":
		for _, kind := range []string{"workload", "profile", "environment", "policy"} {
			dependency, err := resolve(kind, item[kind])
			if err != nil {
				return err
			}
			if kind == "profile" && !reflect.DeepEqual(dependency["workload"], item["workload"]) {
				return errors.New("baseline belongs to another workload")
			}
		}
	}

	return nil
}

func checkWorkload(item map[string]any, resolve resolver) error {
	route, err := resolve("owner-route", item["ownerRoute"])
	if err != nil {
		return err
	}
	if item["state"] == "ACTIVE" && route["state"] != "ACTIVE" {
		return errors.New("active workload has retired owner route")
	}
	seen := map[resourceKey]bool{}
	for _, ref := range item["supportedProfiles"].([]any) {
		profile, err := resolve("profile", ref)
		if err != nil {
			return err
		}
		identity := key("profile", ref)
		if seen[identity] || !reflect.DeepEqual(profile["workload"], item["reference"]) ||
			(item["state"] == "ACTIVE" && profile["state"] != "ACTIVE") {
			return errors.New("unsupported, duplicate or retired workload profile")
		}
		seen[identity] = true
	}

	return nil
}

func checkProfile(item map[string]any, resolve resolver) error {
	workload, err := resolve("workload", item["workload"])
	if err != nil {
		return err
	}
	active, supported := item["state"] == "ACTIVE", false
	for _, ref := range workload["supportedProfiles"].([]any) {
		supported = supported || reflect.DeepEqual(ref, item["reference"])
	}
	if active && (workload["state"] != "ACTIVE" || !supported) {
		return errors.New("active profile has retired or unsupported workload")
	}
	seen := map[string]bool{}
	for _, value := range item["selections"].([]any) {
		selection := object(value)
		identity := string(canonical(selection))
		if seen[identity] {
			return errors.New("duplicate catalog selection")
		}
		seen[identity] = true
		if err := checkSelection(item, selection, active, resolve); err != nil {
			return err
		}
	}

	return nil
}

func checkSelection(profile, selection map[string]any, active bool, resolve resolver) error {
	for _, kind := range []string{"environment", "policy"} {
		dependency, err := resolve(kind, selection[kind])
		if err != nil {
			return err
		}
		if active && dependency["state"] != "ACTIVE" {
			return errors.New("active profile has retired selection dependency")
		}
	}
	if selection["baseline"] == nil {
		return nil
	}
	baseline, err := resolve("baseline", selection["baseline"])
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(baseline["profile"], profile["reference"]) ||
		!reflect.DeepEqual(baseline["workload"], profile["workload"]) ||
		!reflect.DeepEqual(baseline["environment"], selection["environment"]) ||
		!reflect.DeepEqual(baseline["policy"], selection["policy"]) ||
		(active && baseline["state"] != "APPROVED") {
		return errors.New("unapproved or mismatched catalog baseline")
	}

	return nil
}
