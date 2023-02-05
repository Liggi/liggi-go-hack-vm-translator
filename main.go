package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

const locRegister = "@R13"
const valueRegister = "@R14"

type Parser struct{}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("No file specified")
	}

	fileName := os.Args[1]

	// Check first letter of filename is uppercase
	if !strings.HasPrefix(fileName, strings.ToUpper(fileName[:1])) {
		log.Fatal("File must start with an uppercase letter")
	}

	// Check extension is .vm
	if path.Ext(fileName) != ".vm" {
		log.Fatal("File must have .vm extension")
	}

	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	parser := NewParser()

	output, err := parser.Parse(scanner)
	if err != nil {
		log.Fatal(err)
	}

	// Save to file
	extension := path.Ext(fileName)
	outputFilename := strings.TrimSuffix(fileName, extension) + ".asm"
	outputFile, err := os.Create(outputFilename)
	if err != nil {
		log.Fatal(err)
	}

	writer := bufio.NewWriter(outputFile)
	defer writer.Flush()

	for _, instruction := range output {
		writer.WriteString(instruction)
	}
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(scanner *bufio.Scanner) ([]string, error) {
	instructions := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}

		line = strings.Split(line, "//")[0]
		line = strings.TrimSpace(line)

		parts := strings.Fields(line)
		// Reverse the order of the `parts` of each line
		// This is because we process them backwards, ie.
		// push local 5 => 5, passed into `local()`, then we call `push()`
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}

		output, err := parseCommand(parts)
		if err != nil {
			log.Fatal(err)
		}

		instructions = append(instructions, output)
	}

	infiniteLoop := strings.Join([]string{
		"(INFINITE_LOOP)",
		"@INFINITE_LOOP",
		"0;JMP",
	}, "\n")

	instructions = append(instructions, infiniteLoop)

	return instructions, nil
}

func parseCommand(command []string) (string, error) {
	first := command[0]

	// Is the first part of the command a number?
	num, err := strconv.Atoi(first)
	if err == nil {
		// Yes, so we're pushing / popping from the stack
		second := command[1]

		setup, err := location(second, num)
		if err != nil {
			return "", err
		}

		stackOperation, err := stack(command[2])
		if err != nil {
			return "", err
		}

		return setup + "\n" + stackOperation + "\n", nil
	}

	// No, we're running an operation on the stack
	operation, err := operation(first)
	if err != nil {
		return "", err
	}

	return operation + "\n", nil
}

func operation(op string) (string, error) {
	switch op {
	case "add":
		return add(), nil

	case "sub":
		return sub(), nil

	case "neg":
		return neg(), nil

	case "eq":
		return eq(), nil

	case "gt":
		return gt(), nil

	case "lt":
		return lt(), nil

	case "and":
		return and(), nil

	case "or":
		return or(), nil

	case "not":
		return not(), nil

	default:
		return "", fmt.Errorf("invalid operation: %s", op)
	}
}

func location(op string, index int) (string, error) {
	switch op {
	case "argument":
		return argument(index), nil

	case "local":
		return local(index), nil

	case "static":
		return static(index), nil

	case "constant":
		return constant(index), nil

	case "this":
		return this(index), nil

	case "that":
		return that(index), nil

	case "pointer":
		return pointer(index)

	case "temp":
		return temp(index), nil

	default:
		return "", fmt.Errorf("invalid location: %s", op)
	}
}

func stack(op string) (string, error) {
	if op == "push" {
		return push(), nil
	} else if op == "pop" {
		return pop(), nil
	}

	return "", fmt.Errorf("invalid stack operation: %s", op)
}

func push() string {
	lines := []string{
		locRegister,
		"A=M",
		"D=M",
		"@SP",
		"A=M",
		"M=D",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func pop() string {
	lines := []string{
		"@SP",
		"A=M-1",
		"D=M",
		locRegister,
		"A=M",
		"M=D",
	}

	lines = append(lines, decStackPointer())

	return strings.Join(lines, "\n")
}

func local(index int) string {
	if index == 0 {
		return baseLocation("LCL")
	}

	return offsetLocation("LCL", index)
}

func argument(index int) string {
	if index == 0 {
		return baseLocation("ARG")
	}

	return offsetLocation("ARG", index)
}

func pointer(index int) (string, error) {
	if index == 0 {
		return baseLocation("THIS"), nil
	}
	if index == 1 {
		return baseLocation("THAT"), nil
	}

	return "", fmt.Errorf("invalid pointer index: %d", index)
}

func temp(index int) string {
	if index == 0 {
		return baseLocation("R5")
	}

	return offsetLocation("R5", index)
}

func static(index int) string {
	lines := []string{
		fmt.Sprintf("@%s.%d", getFilename(), index),
		"D=A",
		locRegister,
		"M=D",
	}

	return strings.Join(lines, "\n")
}

func constant(index int) string {
	lines := []string{
		fmt.Sprintf("@%d", index),
		"D=A",
		valueRegister,
		"M=D",
		"D=A",
		locRegister,
		"M=D",
	}

	return strings.Join(lines, "\n")
}

func this(index int) string {
	return byPointer("@THIS", index)
}

func that(index int) string {
	return byPointer("@THAT", index)
}

func byPointer(location string, index int) string {
	var lines []string

	if index == 0 {
		lines = []string{
			location,
			"A=M",
			"D=A",
			locRegister,
			"M=D",
		}
	} else {
		lines = []string{
			fmt.Sprintf("@%d", index),
			"D=A",
			location,
			"A=M",
			"D=D+A",
			locRegister,
			"M=D",
		}
	}

	return strings.Join(lines, "\n")
}

func add() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"M=D+M",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func sub() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"M=M-D",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func neg() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"M=-M",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

var eqCount = 0

func eq() string {
	eqTrueLabel := fmt.Sprintf("EQ_TRUE_%d", eqCount)
	eqEndLabel := fmt.Sprintf("EQ_END_%d", eqCount)

	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		fmt.Sprintf("@%s", eqTrueLabel),
		"D;JEQ",
		"@SP",
		"A=M",
		"M=0",
		fmt.Sprintf("@%s", eqEndLabel),
		"0;JMP",
		fmt.Sprintf("(%s)", eqTrueLabel),
		"@SP",
		"A=M",
		"M=-1",
		fmt.Sprintf("(%s)", eqEndLabel),
	}

	eqCount++

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

var gtCount = 0

func gt() string {
	gtTrueLabel := fmt.Sprintf("GT_TRUE_%d", gtCount)
	gtEndLabel := fmt.Sprintf("GT_END_%d", gtCount)

	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		fmt.Sprintf("@%s", gtTrueLabel),
		"D;JGT",
		"@SP",
		"A=M",
		"M=0",
		fmt.Sprintf("@%s", gtEndLabel),
		"0;JMP",
		fmt.Sprintf("(%s)", gtTrueLabel),
		"@SP",
		"A=M",
		"M=-1",
		fmt.Sprintf("(%s)", gtEndLabel),
	}

	gtCount++

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

var ltCount = 0

func lt() string {
	ltTrueLabel := fmt.Sprintf("LT_TRUE_%d", ltCount)
	ltEndLabel := fmt.Sprintf("LT_END_%d", ltCount)

	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		fmt.Sprintf("@%s", ltTrueLabel),
		"D;JLT",
		"@SP",
		"A=M",
		"M=0",
		fmt.Sprintf("@%s", ltEndLabel),
		"0;JMP",
		fmt.Sprintf("(%s)", ltTrueLabel),
		"@SP",
		"A=M",
		"M=-1",
		fmt.Sprintf("(%s)", ltEndLabel),
	}

	ltCount++

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func and() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"M=D&M",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func or() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"M=D|M",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func not() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"M=!M",
	}

	lines = append(lines, incStackPointer())

	return strings.Join(lines, "\n")
}

func baseLocation(location string) string {
	lines := []string{
		fmt.Sprintf("@%s", location),
		"D=A",
		locRegister,
		"M=D",
	}

	return strings.Join(lines, "\n")
}

func offsetLocation(location string, index int) string {
	lines := []string{
		fmt.Sprintf("@%d", index),
		"D=A",
		fmt.Sprintf("@%s", location),
		"D=D+A",
		locRegister,
		"M=D",
	}

	return strings.Join(lines, "\n")
}

func incStackPointer() string {
	lines := []string{
		"@SP",
		"M=M+1",
	}

	return strings.Join(lines, "\n")
}

func decStackPointer() string {
	lines := []string{
		"@SP",
		"M=M-1",
	}

	return strings.Join(lines, "\n")
}

func getFilename() string {
	fileName := path.Base(os.Args[1])
	fileName = strings.TrimSuffix(fileName, path.Ext(fileName))

	return fileName
}
