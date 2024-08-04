package utils

import (
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func GetAllRegexNamespaces(namespaces *v1.NamespaceList, pattern string) []v1.Namespace {
	// match with regex
	if pattern == "*" {
		return namespaces.Items
	}
	var matchedNamespaces []v1.Namespace
	for _, namespace := range namespaces.Items {
		matched, err := regexp.MatchString(pattern, namespace.Name)
		if err != nil {
			panic(err.Error())
		}
		if matched {
			matchedNamespaces = append(matchedNamespaces, namespace)
		}
	}
	return matchedNamespaces
}

// Evaluate the regex in annotation and return a list of all namespaces that configmap is needed to replicate to
func GetReplicateNamespaces(allNamespaces *v1.NamespaceList, replicateAnnotationString string) ([]string, error) {
	// output as a set of strings
	// we use maps to implement sets
	replicateNamespaceSet := map[string]struct{}{}

	// evaluate the regex on the namespace
	// append the names of the matched namespaces to output
	patterns := strings.Split(replicateAnnotationString, ",")
	for _, pattern := range patterns {
		namespaces := GetAllRegexNamespaces(allNamespaces, pattern)
		for _, namespace := range namespaces {
			replicateNamespaceSet[namespace.Name] = struct{}{}
		}
	}

	keys := make([]string, 0, len(replicateNamespaceSet))
	for k := range replicateNamespaceSet {
		keys = append(keys, k)
	}

	return keys, nil
}

func NameInStringArray(name string, arr []string) bool {
	for _, val := range arr {
		if name == val {
			return true
		}
	}
	return false
}
