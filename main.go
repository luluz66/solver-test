package main

import (
	"fmt"
	"log"

	cmpb "github.com/google/or-tools/ortools/sat/proto/cpmodel"

	"github.com/google/or-tools/ortools/sat/go/cpmodel"
)

func main() {
	model := cpmodel.NewCpModelBuilder()

	x := model.NewIntVar(1, 3)
	y := model.NewIntVar(1, 3)
	b := model.NewBoolVar()

	model.AddLessOrEqual(x, cpmodel.NewConstant(1)).OnlyEnforceIf(b)
	model.AddLessOrEqual(y, cpmodel.NewConstant(1)).OnlyEnforceIf(b.Not())

	obj := cpmodel.NewLinearExpr().AddSum(x, b.Not()).AddTerm(y, 5)
	model.Maximize(obj)
	m, err := model.Model()
	if err != nil {
		log.Fatalf("Building model returned with error %v", err)
	}
	res, err := cpmodel.SolveCpModel(m)
	if err != nil {
		log.Fatalf("CP solver returned with unexpected err %v", err)
	}
	if res.GetStatus() != cmpb.CpSolverStatus_FEASIBLE && res.GetStatus() != cmpb.CpSolverStatus_OPTIMAL {
		log.Fatalf("CP solver returned with status %v", res.GetStatus())
	}

	fmt.Println("Objective:", res.GetObjectiveValue())
	fmt.Println("x:", cpmodel.SolutionIntegerValue(res, x))
	fmt.Println("y:", cpmodel.SolutionIntegerValue(res, y))
	fmt.Println("Int b:", cpmodel.SolutionIntegerValue(res, b))
	fmt.Println("Bool b:", cpmodel.SolutionBooleanValue(res, b))

}
