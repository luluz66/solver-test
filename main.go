package main

import (
	"fmt"
	"log"
	"slices"

	cmpb "github.com/google/or-tools/ortools/sat/proto/cpmodel"

	"github.com/google/or-tools/ortools/sat/go/cpmodel"
)

const (
	usWest1a = "us-west-1a"
	usWest1b = "us-west-1b"
	usWest1c = "us-west-1c"
)

var (
	regions = []string{usWest1a, usWest1b, usWest1c}
)

type Node struct {
	hasRange     bool
	replicaCount int64
	region       string
}

type region struct {
	inRegionBefore []bool
	inRegionAfter  cpmodel.BoolVar
	inRegionExpr   *cpmodel.LinearExpr
}

func solveModel(b *cpmodel.Builder) (*cmpb.CpSolverResponse, error) {
	m, err := b.Model()
	if err != nil {
		return nil, fmt.Errorf("failed to build model: %s", err)
	}

	res, err := cpmodel.SolveCpModel(m)
	if err != nil {
		return nil, fmt.Errorf("failed to solve model: %s", err)
	}

	if res.GetStatus() != cmpb.CpSolverStatus_FEASIBLE && res.GetStatus() != cmpb.CpSolverStatus_OPTIMAL {
		return nil, fmt.Errorf("CP solver returned with status %v", res.GetStatus())
	}
	return res, nil
}

func solve(nodes []*Node) error {
	numNodes := len(nodes)
	numRegions := len(regions)
	replicaCounts := make([]int64, 0, numNodes)
	regionMap := make(map[string]*region, numRegions)

	model := cpmodel.NewCpModelBuilder()

	for _, r := range regions {
		regionMap[r] = &region{
			inRegionBefore: make([]bool, 0, numNodes),
			inRegionAfter:  model.NewBoolVar().WithName(fmt.Sprintf("replica_in_region_%s", r)),
			inRegionExpr:   cpmodel.NewLinearExpr(),
		}
	}
	for _, n := range nodes {
		replicaCounts = append(replicaCounts, n.replicaCount)
		for _, r := range regions {
			regionMap[r].inRegionBefore = append(regionMap[r].inRegionBefore, n.region == r)
		}
	}
	replicaCountUpperBound := int64(slices.Max(replicaCounts))

	// ops[i] represents the operation on nodes[i]
	// -1 indicates removing a replica.
	// 0 indicates no-op.
	// +1 indicates adding a replica.
	ops := make([]cpmodel.IntVar, 0, numNodes)
	replicaCountsAfter := make([]cpmodel.IntVar, 0, numNodes)

	moved := make([]cpmodel.BoolVar, 0, numNodes)
	opsSum := cpmodel.NewLinearExpr()
	movedSum := cpmodel.NewLinearExpr()

	for i, n := range nodes {
		op := model.NewIntVar(-1, 1).WithName(fmt.Sprintf("op_%d", i))
		movedVar := model.NewBoolVar().WithName(fmt.Sprintf("moved_%d", i))
		ops = append(ops, op)
		moved = append(moved, movedVar)
		opsSum.Add(op)
		movedSum.Add(movedVar)
		// if !moved, ops[i] == 0
		// if moved, ops[i] != 0
		model.AddEquality(op, cpmodel.NewConstant(0)).OnlyEnforceIf(movedVar.Not())
		model.AddNotEqual(op, cpmodel.NewConstant(0)).OnlyEnforceIf(movedVar)

		replicaCountAfter := model.NewIntVar(0, replicaCountUpperBound).WithName(fmt.Sprintf("replica_count_after_%d", i))
		replicaCountsAfter = append(replicaCountsAfter, replicaCountAfter)
		// replicaCountAfter = n.replicaCount + op[i]
		model.AddEquality(replicaCountAfter, cpmodel.NewLinearExpr().AddConstant(n.replicaCount).Add(ops[i]))
		model.AddGreaterOrEqual(replicaCountsAfter[i], model.NewConstant(0))

		// Each node can have at most one replica of this range;
		// node.has_range + op[i] > =0
		expr := cpmodel.NewLinearExpr().Add(op)
		if n.hasRange {
			expr.AddConstant(1)
		}
		model.AddGreaterOrEqual(expr, cpmodel.NewConstant(0))

		for _, r := range regionMap {
			if r.inRegionBefore[i] {
				// r.inRegionExpr += (n.hasRange + ops[i])

				r.inRegionExpr.Add(op)
				if n.hasRange {
					r.inRegionExpr.AddConstant(1)
				}
			}
		}
	}

	// The total number of replicas are the same, i.e. sum(ops) == 0
	model.AddEquality(opsSum, cpmodel.NewConstant(0))

	// At most two operations (one remove and one add) is allowed
	// i.e. sum(moved) <=2
	model.AddLessOrEqual(movedSum, cpmodel.NewConstant(2))

	diversity := cpmodel.NewLinearExpr()
	for _, r := range regionMap {
		model.AddGreaterThan(r.inRegionExpr, cpmodel.NewConstant(0)).OnlyEnforceIf(r.inRegionAfter)
		model.AddEquality(r.inRegionExpr, cpmodel.NewConstant(0)).OnlyEnforceIf(r.inRegionAfter.Not())
		diversity.Add(r.inRegionAfter)
	}

	model.Maximize(diversity)
	res, err := solveModel(model)
	if err != nil {
		return fmt.Errorf("first phase failed: %s", err)
	}

	log.Printf("Wall time: %.5f\n", res.GetWallTime())
	log.Printf("Objective: %.2f\n", res.GetObjectiveValue())

	hint := &cpmodel.Hint{
		Ints: make(map[cpmodel.IntVar]int64),
	}
	for i := range nodes {
		hint.Ints[ops[i]] = cpmodel.SolutionIntegerValue(res, ops[i])
	}

	replicaCountsAfterLinearArguments := make([]cpmodel.LinearArgument, 0, len(replicaCountsAfter))
	for _, rc := range replicaCountsAfter {
		replicaCountsAfterLinearArguments = append(replicaCountsAfterLinearArguments, cpmodel.LinearArgument(rc))
	}

	model.SetHint(hint)
	model.AddEquality(diversity, cpmodel.NewConstant(int64(res.GetObjectiveValue())))
	maxReplicaCount := model.NewIntVar(0, replicaCountUpperBound).WithName("max_replica_count")
	model.AddMaxEquality(maxReplicaCount, replicaCountsAfterLinearArguments...)
	minReplicaCount := model.NewIntVar(0, replicaCountUpperBound).WithName("min_replica_count")
	model.AddMinEquality(minReplicaCount, replicaCountsAfterLinearArguments...)
	// min(maxReplicaCount - minReplicaCount)
	model.Minimize(cpmodel.NewLinearExpr().Add(maxReplicaCount).AddTerm(minReplicaCount, -1))

	res, err = solveModel(model)
	if err != nil {
		return fmt.Errorf("second phase failed: %s", err)
	}
	log.Printf("Wall time: %.5f\n", res.GetWallTime())
	log.Printf("Objective: %.2f\n", res.GetObjectiveValue())
	for i := range nodes {
		fmt.Printf("%s: %d\n", ops[i].Name(), cpmodel.SolutionIntegerValue(res, ops[i]))
		fmt.Printf("%s: %d\n", replicaCountsAfter[i].Name(), cpmodel.SolutionIntegerValue(res, replicaCountsAfter[i]))
	}
	for _, r := range regionMap {
		fmt.Printf("%s: %t\n", r.inRegionAfter.Name(), cpmodel.SolutionBooleanValue(res, r.inRegionAfter))
	}
	return nil
}

func main() {
	nodes := []*Node{
		&Node{hasRange: true, replicaCount: 6, region: usWest1a},
		&Node{hasRange: false, replicaCount: 2, region: usWest1a},
		&Node{hasRange: true, replicaCount: 2, region: usWest1a},
		&Node{hasRange: true, replicaCount: 6, region: usWest1b},
		&Node{hasRange: false, replicaCount: 6, region: usWest1b},
		&Node{hasRange: false, replicaCount: 3, region: usWest1b},
		&Node{hasRange: false, replicaCount: 3, region: usWest1c},
		&Node{hasRange: false, replicaCount: 3, region: usWest1c},
		&Node{hasRange: false, replicaCount: 3, region: usWest1c},
	}
	err := solve(nodes)
	if err != nil {
		log.Printf("failed to solve: %s", err)
	}
}
