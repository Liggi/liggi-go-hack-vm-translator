package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const locRegister = "@R13"
const valueRegister = "@R14"

type Parser struct{}

type Stack struct {
	items         []string
	returnCounter int
}

var funcStack = Stack{
	items:         []string{"Sys.init"},
	returnCounter: 0,
}

func main() {
	var instructions []string
	var filename string
	var err error

	if len(os.Args) != 2 {
		log.Fatal("no file or folder specified")
	}

	arg := os.Args[1]
	ext := path.Ext(arg)

	if ext == ".vm" {
		instructions, err = parseFile(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}

		filename = strings.TrimSuffix(arg, ext) + ".asm"
	} else if ext == "" {
		instructions, err = loadFolder(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}

		filename = getFolderName() + ".asm"
	}

	save(instructions, filename)
}

func save(instructions []string, fileName string) {
	// Save to file
	extension := path.Ext(fileName)
	outputFilename := strings.TrimSuffix(fileName, extension) + ".asm"
	outputFile, err := os.Create(outputFilename)
	if err != nil {
		log.Fatal(err)
	}

	writer := bufio.NewWriter(outputFile)
	defer writer.Flush()

	for _, instruction := range instructions {
		writer.WriteString(instruction)
	}
}

func loadFolder(folderName string) ([]string, error) {
	// If not, look for `.vm` files within the current folder and translate all of them
	files, err := filepath.Glob("*.vm")
	if err != nil {
		log.Fatal(err)
	}

	if len(files) == 0 {
		log.Fatal("no .vm files found in folder")
	}

	instructions := []string{}

	// If the user has passed `--bootstrap`, add the bootstrap code
	shouldBootstrap := flag.Bool("bootstrap", false, "include bootstrapping instructions")
	flag.Parse()
	if *shouldBootstrap {
		bootstrap := strings.Join([]string{
			"@256",
			"D=A",
			"@SP",
			"M=D",
			fmt.Sprintf("@%s.Sys.init", getFolderName()),
			"0;JMP",
		}, "\n") + "\n"

		instructions = append(instructions, bootstrap)
	}

	for _, file := range files {
		lines, err := parseFile(file)
		if err != nil {
			log.Fatal(err)
		}

		instructions = append(instructions, lines...)
	}

	return instructions, nil
}

var currentFile string

func parseFile(fileName string) ([]string, error) {
	// Check first letter of filename is uppercase
	if !strings.HasPrefix(fileName, strings.ToUpper(fileName[:1])) {
		log.Fatal("file must start with an uppercase letter")
	}

	// Check extension is .vm
	if path.Ext(fileName) != ".vm" {
		log.Fatal("file must have .vm extension")
	}

	currentFile = fileName

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

	return output, nil
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

		output, err := parseCommand(line)
		if err != nil {
			log.Fatal(err)
		}

		instructions = append(instructions, "// "+line+"\n")
		instructions = append(instructions, output)
	}

	endWithLoop := flag.Bool("endWithLoop", false, "end with infinite loop")
	flag.Parse()
	if *endWithLoop {
		infiniteLoop := strings.Join([]string{
			"(INFINITE_LOOP)",
			"@INFINITE_LOOP",
			"0;JMP",
		}, "\n") + "\n"

		instructions = append(instructions, infiniteLoop)
	}

	return instructions, nil
}

func parseCommand(line string) (string, error) {
	command := strings.Fields(line)

	first := command[0]

	switch first {
	case "function":
		return function(command[1], command[2])

	case "call":
		return callFunction(command[1], command[2])

	case "return":
		return returnFromFunction(), nil

	case "goto":
		return gotoLabel(command[1]), nil

	case "if-goto":
		return ifGoto(command[1]), nil

	case "label":
		return label(command[1]), nil
	}

	// If none of the above, it's either a push / pop command, or a single-part operation command

	// Is this a single-part command? (ie. an operation)
	if len(command) == 1 {
		operation, err := operation(command[0])
		if err != nil {
			return "", err
		}

		return operation, nil
	}

	// Is the third part of the command a number?
	num, err := strconv.Atoi(command[2])
	if err == nil {
		// Yes, so we're pushing / popping from the stack
		second := command[1]

		setup, err := location(second, num)
		if err != nil {
			return "", err
		}

		stackOperation, err := stack(command[0])
		if err != nil {
			return "", err
		}

		return setup + "\n" + stackOperation, nil
	}

	return "", fmt.Errorf("invalid command: %s", command)
}

func function(name string, nVars string) (string, error) {
	numVars, err := strconv.Atoi(nVars)
	if err != nil {
		return "", fmt.Errorf("invalid vars for function definition (%s): %s", name, nVars)
	}

	// Change the function context
	funcStack.Push(name)

	// Initialise all local variables to 0
	lines := []string{
		fmt.Sprintf("(%s)", getFolderName()+"."+name),
	}

	initLocalVariable := []string{
		"@SP",
		"A=M",
		"M=0",
		"@SP",
		"M=M+1",
	}

	for i := 0; i < numVars; i++ {
		lines = append(lines, initLocalVariable...)
	}

	return strings.Join(lines, "\n"), nil
}

func callFunction(name string, nArgs string) (string, error) {
	numArgs, err := strconv.Atoi(nArgs)
	if err != nil {
		return "", fmt.Errorf("invalid args to function (%s): %s", name, nArgs)
	}

	callingFuncName := funcStack.Peek()
	returnLabel := getFolderName() + "." + callingFuncName + "$ret" + strconv.Itoa(funcStack.returnCounter)

	lines := []string{
		// Push the return address onto the stack
		fmt.Sprintf("@%s", returnLabel),
		"D=A",
		"@SP",
		"A=M",
		"M=D",
		"@SP",
		"M=M+1",

		// Push LCL onto the stack
		"@LCL",
		"D=M",
		"@SP",
		"A=M",
		"M=D",
		"@SP",
		"M=M+1",

		// Push ARG onto the stack
		"@ARG",
		"D=M",
		"@SP",
		"A=M",
		"M=D",
		"@SP",
		"M=M+1",

		// Push THIS onto the stack
		"@THIS",
		"D=M",
		"@SP",
		"A=M",
		"M=D",
		"@SP",
		"M=M+1",

		// Push THAT onto the stack
		"@THAT",
		"D=M",
		"@SP",
		"A=M",
		"M=D",
		"@SP",
		"M=M+1",

		// Set new ARG
		"@SP",
		"D=M",
		fmt.Sprintf("@%d", numArgs),
		"D=D-A",
		"@5",
		"D=D-A",
		"@ARG",
		"M=D",

		// Set up new LCL
		"@SP",
		"D=M",
		"@LCL",
		"M=D",

		// Jump to the function
		fmt.Sprintf("@%s", getFolderName()+"."+name),
		"0;JMP",

		// Set the return label for this call
		fmt.Sprintf("(%s)", returnLabel),
	}

	// Increment the return counter for the next call from this function
	funcStack.returnCounter++

	return strings.Join(lines, "\n"), nil
}

func returnFromFunction() string {
	lines := []string{
		// Put the return address in the location register
		"@LCL",
		"D=M",
		"@5",
		"D=D-A",
		"A=D",
		"D=M",
		locRegister,
		"M=D",

		// Take the top of the working stack and put it at @ARG
		"@SP",
		"M=M-1",
		"A=M",
		"D=M",
		"@ARG",
		"A=M",
		"M=D",

		// Move the stack pointer
		"@ARG",
		"D=M+1",
		"@SP",
		"M=D",

		// Restore THAT
		"@LCL",
		"A=M-1",
		"D=M",
		"@THAT",
		"M=D",

		// Restore THIS
		"@LCL",
		"D=M",
		"@2",
		"D=D-A",
		"A=D",
		"D=M",
		"@THIS",
		"M=D",

		// Restore ARG
		"@LCL",
		"D=M",
		"@3",
		"D=D-A",
		"A=D",
		"D=M",
		"@ARG",
		"M=D",

		// Restore LCL
		"@LCL",
		"D=M",
		"@4",
		"D=D-A",
		"A=D",
		"D=M",
		"@LCL",
		"M=D",

		// Jump to the return address
		locRegister,
		"A=M",
		"0;JMP",
	}
	// Change the function context
	funcStack.Pop()

	return strings.Join(lines, "\n")
}

func gotoLabel(label string) string {
	lines := []string{
		fmt.Sprintf("@%s", label),
		"0;JMP",
	}

	return strings.Join(lines, "\n") + "\n"
}

func ifGoto(label string) string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		fmt.Sprintf("@%s", label),
		"D;JNE",
	}

	return strings.Join(lines, "\n") + "\n"
}

func label(label string) string {
	return fmt.Sprintf("(%s)", label) + "\n"
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
	return byPointer("@LCL", index)
}

func argument(index int) string {
	return byPointer("@ARG", index)
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

	lines := []string{
		fmt.Sprintf("@%d", index),
		"D=A",
		"@R5",
		"D=D+A",
		locRegister,
		"M=D",
	}

	return strings.Join(lines, "\n")
}

func static(index int) string {
	lines := []string{
		fmt.Sprintf("@%s.%d", currentFile, index),
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

func getFolderName() string {
	// Get the name of the current folder
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	return path.Base(dir)
}

func (s *Stack) Push(item string) {
	s.items = append(s.items, item)
}

func (s *Stack) Pop() {
	s.items = s.items[:len(s.items)-1]
}

func (s *Stack) Peek() string {
	return s.items[len(s.items)-1]
}
