package graph

import "fmt"

func (g *Graph) TopologicalSort() ([]string, error) {
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

	order := make([]string, 0, len(g.nodes))
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		order = append(order, node)

		for _, to := range g.edges[node] {
			inDegree[to]--
			if inDegree[to] == 0 {
				queue = append(queue, to)
			}
		}
	}

	if len(order) != len(g.nodes) {
		stuck := make([]string, 0)
		for id, degree := range inDegree {
			if degree > 0 {
				stuck = append(stuck, id)
			}
		}
		return nil, fmt.Errorf("circular dependency detected (stuck nodes: %v)", stuck)
	}
	return order, nil
}

func (g *Graph) TopologicalSortReverse() ([]string, error) {
	normal, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	// Reverse the slice
	for i, j := 0, len(normal)-1; i < j; i, j = i+1, j-1 {
		normal[i], normal[j] = normal[j], normal[i]
	}

	return normal, nil
}
