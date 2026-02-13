package graph

import (
	"GoIaC/pkg/config"
	"fmt"
	"regexp"
)

// refPattern matches ${type.resource_id.attribute} interpolation expressions
var refPattern = regexp.MustCompile(`\$\{(\w+)\.(\w+)\.(\w+)\}`)

type Graph struct {
	nodes map[string]*Node
	edges map[string][]string
}

type Node struct {
	ID       string
	Type     string
	Resource *config.Resource
}

func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
		edges: make(map[string][]string),
	}
}

func (g *Graph) Build(resources []*config.Resource) error {
	for _, resource := range resources {
		g.nodes[resource.ID] = &Node{
			ID:       resource.ID,
			Type:     resource.Type,
			Resource: resource,
		}
	}

	for _, resource := range resources {
		refs := ExtractReferences(resource.Properties)
		for _, depID := range refs {
			g.edges[resource.ID] = append(g.edges[resource.ID], depID)
		}
	}
	return nil
}

// ExtractReferences TODO: remove recursion
func ExtractReferences(properties map[string]interface{}) []string {
	refs := []string{}
	seen := make(map[string]bool)

	var scan func(interface{})
	scan = func(v interface{}) {
		switch val := v.(type) {
		case string:
			matches := refPattern.FindAllStringSubmatch(val, -1)
			for _, match := range matches {
				resourceID := match[2]
				if !seen[resourceID] {
					refs = append(refs, resourceID)
					seen[resourceID] = true
				}
			}
		case map[string]interface{}:
			for _, nested := range val {
				scan(nested)
			}
		case []interface{}:
			for _, item := range val {
				scan(item)
			}
		}
	}

	for _, v := range properties {
		scan(v)
	}
	return refs
}

// ValidateDAG checks for circular dependencies
func (g *Graph) ValidateDAG() error {
	inDegree := make(map[string]int, len(g.nodes))
	for _, node := range g.nodes {
		inDegree[node.ID] = 0
	}

	for from, tos := range g.edges {
		if _, ok := g.nodes[from]; !ok {
			continue
		}
		for _, to := range tos {
			if _, ok := g.nodes[to]; !ok {
				continue
			}
			inDegree[to]++
		}
	}

	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	processed := 0
	for head := 0; head < len(queue); head++ {
		n := queue[head]
		processed++

		for _, to := range g.edges[n] {
			if _, ok := g.nodes[to]; !ok {
				continue
			}
			inDegree[to]--
			if inDegree[to] == 0 {
				queue = append(queue, to)
			}
		}
	}

	if processed != len(g.nodes) {
		stuck := make([]string, 0)
		for id, degree := range inDegree {
			if degree > 0 {
				stuck = append(stuck, id)
			}
		}
		return fmt.Errorf("circular dependency detected (stuck nodes: %v)", stuck)
	}

	return nil
}

// ValidateReferences checks that all referenced resources exist
func (g *Graph) ValidateReferences() error {
	for node, deps := range g.edges {
		for _, dep := range deps {
			if _, exists := g.nodes[dep]; !exists {
				return fmt.Errorf("resource %s references undefined resource %s", node, dep)
			}
		}
	}
	return nil
}

func (g *Graph) GetDependencies(nodeID string) []string {
	return g.edges[nodeID]
}

func (g *Graph) GetNodes() map[string]*Node {
	return g.nodes
}
