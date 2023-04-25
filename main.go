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

var shouldBootstrap bool
var shouldEndWithLoop bool
var shouldSetStackPointer bool

var pathToTranslate string

const locRegister = "@R13"
const valueRegister = "@R14"

type Parser struct{}

type Stack struct {
	current       string
	returnCounter int
}

var funcStack = Stack{
	current:       "Sys.init",
	returnCounter: 0,
}

func main() {
	var instructions []string
	var filename string
	var err error

	bootstrap := flag.Bool("bootstrap", false, "include bootstrapping instructions")
	setStackPointer := flag.Bool("setStackPointer", false, "set the stack pointer to 256")
	endWithLoop := flag.Bool("endWithLoop", false, "end with infinite loop")
	passedPath := flag.String("path", "", "path to folder or file to translate")
	flag.Parse()

	shouldBootstrap = *bootstrap
	shouldSetStackPointer = *setStackPointer
	shouldEndWithLoop = *endWithLoop
	pathToTranslate = *passedPath

	if pathToTranslate == "" {
		log.Fatal("no file or folder specified")
	}

	ext := path.Ext(pathToTranslate)

	if ext == ".vm" {
		instructions, err = parseFile(pathToTranslate)
		if err != nil {
			log.Fatal(err)
		}

		filename = strings.TrimSuffix(pathToTranslate, ext) + ".asm"
	} else if ext == "" {
		instructions, err = loadFolder(pathToTranslate)
		if err != nil {
			log.Fatal(err)
		}

		filename = getFolderName() + ".asm"
	} else {
		log.Fatal("invalid file extension")
	}

	save(instructions, filename)
}

func save(instructions []string, fileName string) {
	var saveToFolderPath string

	info, err := os.Stat(pathToTranslate)
	if err != nil {
		fmt.Println(err)
		return
	}

	if info.IsDir() {
		saveToFolderPath = pathToTranslate
	} else {
		saveToFolderPath = filepath.Dir(pathToTranslate)
	}

	// Save to file
	extension := path.Ext(fileName)
	outputFilename := strings.TrimSuffix(fileName, extension) + ".asm"
	//fmt.Println(pathToSave + "/" + outputFilename)
	outputFile, err := os.Create(saveToFolderPath + "/" + outputFilename)
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
	files, err := filepath.Glob(folderName + "/*.vm")
	if err != nil {
		log.Fatal(err)
	}

	if len(files) == 0 {
		log.Fatal("no .vm files found in folder")
	}

	instructions := []string{
		"(START)\n",
	}

	if shouldBootstrap {
		init, err := callFunction("Sys.init", "0")
		if err != nil {
			log.Fatal(err)
		}

		instructions = append(instructions, init)
	}

	for _, file := range files {
		lines, err := parseFile(file)
		if err != nil {
			log.Fatal(err)
		}

		instructions = append(instructions, lines...)
	}

	// Needs to go here instead
	instructions = prependFunctions(instructions)
	instructions = prependStartInstructions(instructions)

	if shouldEndWithLoop {
		infiniteLoop := strings.Join([]string{
			"(INFINITE_LOOP)",
			"@INFINITE_LOOP",
			"0;JMP",
		}, "\n") + "\n"

		instructions = append(instructions, infiniteLoop)
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

	currentFile = filepath.Base(fileName)

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

func prependFunctions(instructions []string) []string {
	// Prepend the functions
	functions := createReturnRoutine()
	functions = append(functions, createCallRoutine()...)
	functions = append(functions, createLtRoutine()...)
	functions = append(functions, createGtRoutine()...)
	functions = append(functions, createEqRoutine()...)

	return append(functions, instructions...)
}

func createReturnRoutine() []string {
	returnFunction := strings.Join([]string{
		"(RETURN)",

		// Put the return address in the location register
		"@5",
		"D=A",
		"@LCL",
		"A=M-D",
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
	}, "\n") + "\n"

	return []string{returnFunction}
}

func createCallRoutine() []string {
	callFunction := strings.Join([]string{
		"(CALL)",

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

		// Set new ARG (numArgs is the value of the valueRegister)
		"@SP",
		"D=M",
		valueRegister,
		"D=D-M",
		"@5",
		"D=D-A",
		"@ARG",
		"M=D",

		// Set up new LCL
		"@SP",
		"D=M",
		"@LCL",
		"M=D",

		// Get the function from the locRegister and jump to it
		locRegister,
		"A=M",
		"0;JMP",
	}, "\n") + "\n"

	return []string{callFunction}
}

func createLtRoutine() []string {
	ltFunction := strings.Join([]string{
		"(LT)",
		"@R15",
		"M=D",

		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		"M=0",
		"@END_LT",
		"D;JGE",

		"@SP",
		"A=M",
		"M=-1",

		"(END_LT)",

		"@SP",
		"M=M+1",

		"@R15",
		"A=M",
		"0;JMP",
	}, "\n") + "\n"

	return []string{ltFunction}
}

func createGtRoutine() []string {
	gtFunction := strings.Join([]string{
		"(GT)",
		"@R15",
		"M=D",

		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		"M=0",
		"@END_GT",
		"D;JLE",

		"@SP",
		"A=M",
		"M=-1",

		"(END_GT)",

		"@SP",
		"M=M+1",

		"@R15",
		"A=M",
		"0;JMP",
	}, "\n") + "\n"

	return []string{gtFunction}
}

func createEqRoutine() []string {
	eqFunction := strings.Join([]string{
		"(EQ)",
		"@R15",
		"M=D",

		"@SP",
		"AM=M-1",
		"D=M",
		"@SP",
		"AM=M-1",
		"D=M-D",
		"M=0",
		"@END_EQ",
		"D;JNE",

		"@SP",
		"A=M",
		"M=-1",

		"(END_EQ)",

		"@SP",
		"M=M+1",

		"@R15",
		"A=M",
		"0;JMP",
	}, "\n") + "\n"

	return []string{eqFunction}
}

func prependStartInstructions(instructions []string) []string {
	setStackPointer := strings.Join([]string{
		"@256",
		"D=A",
		"@SP",
		"M=D",
	}, "\n") + "\n"

	start := strings.Join([]string{
		"@START",
		"0;JMP",
	}, "\n") + "\n"

	if shouldSetStackPointer {
		return append([]string{setStackPointer, start}, instructions...)
	}

	return append([]string{start}, instructions...)
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

		instructions = append(instructions, output)
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

		return operation + "\n", nil
	}

	// Is the third part of the command a number?
	num, err := strconv.Atoi(command[2])
	if err == nil {
		// Yes, so we're pushing / popping from the stack
		second := command[1]

		if command[0] == "push" {
			return handlePush(second, num), nil
		} else if command[0] == "pop" {
			return handlePop(second, num), nil
		}

		return "", fmt.Errorf("invalid command: %s", command)
	}

	return "", fmt.Errorf("invalid command: %s", command)
}

func handlePush(segment string, index int) string {
	var lines []string

	switch segment {
	case "constant":
		lines = []string{
			fmt.Sprintf("@%d", index),
			"D=A",
			"@SP",
			"AM=M+1",
			"A=A-1",
			"M=D",
		}

	case "argument":
		if index == 0 {
			lines = []string{
				"@ARG",
				"A=M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@ARG",
				"A=M",
				"D=D+A",
				"A=D",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		}

	case "local":
		if index == 0 {
			lines = []string{
				"@LCL",
				"A=M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@LCL",
				"A=M",
				"D=D+A",
				"A=D",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		}

	case "static":
		lines = []string{
			fmt.Sprintf("@%s.%d", currentFile, index),
			"D=M",
			"@SP",
			"AM=M+1",
			"A=A-1",
			"M=D",
		}

	case "this":
		if index == 0 {
			lines = []string{
				"@THIS",
				"A=M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@THIS",
				"A=D+M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		}

	case "that":
		if index == 0 {
			lines = []string{
				"@THAT",
				"A=M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@THAT",
				"A=D+M",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		}

	case "pointer":
		if index == 0 {
			lines = []string{
				"@THIS",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		} else if index == 1 {
			lines = []string{
				"@THAT",
				"D=M",
				"@SP",
				"AM=M+1",
				"A=A-1",
				"M=D",
			}
		}

	case "temp":
		lines = []string{
			fmt.Sprintf("@%d", index+5),
			"D=M",
			"@SP",
			"AM=M+1",
			"A=A-1",
			"M=D",
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func handlePop(segment string, index int) string {
	var lines []string

	switch segment {
	case "argument":
		if index == 0 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@ARG",
				"A=M",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@ARG",
				"A=D+M",
				"D=A",
				locRegister,
				"M=D",

				"@SP",
				"AM=M-1",
				"D=M",
				locRegister,
				"A=M",
				"M=D",
			}
		}

	case "local":
		if index == 0 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@LCL",
				"A=M",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@LCL",
				"A=D+M",
				"D=A",
				locRegister,
				"M=D",

				"@SP",
				"AM=M-1",
				"D=M",
				locRegister,
				"A=M",
				"M=D",
			}
		}

	case "static":
		lines = []string{
			"@SP",
			"AM=M-1",
			"D=M",
			fmt.Sprintf("@%s.%d", currentFile, index),
			"M=D",
		}

	case "this":
		if index == 0 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@THIS",
				"A=M",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@THIS",
				"A=D+M",
				"D=A",
				locRegister,
				"M=D",

				"@SP",
				"AM=M-1",
				"D=M",
				locRegister,
				"A=M",
				"M=D",
			}
		}

	case "that":
		if index == 0 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@THAT",
				"A=M",
				"M=D",
			}
		} else {
			lines = []string{
				fmt.Sprintf("@%d", index),
				"D=A",
				"@THAT",
				"A=D+M",
				"D=A",
				locRegister,
				"M=D",

				"@SP",
				"AM=M-1",
				"D=M",
				locRegister,
				"A=M",
				"M=D",
			}
		}

	case "pointer":
		if index == 0 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@THIS",
				"M=D",
			}
		} else if index == 1 {
			lines = []string{
				"@SP",
				"AM=M-1",
				"D=M",
				"@THAT",
				"M=D",
			}
		}

	case "temp":
		lines = []string{
			"@SP",
			"AM=M-1",
			"D=M",
			fmt.Sprintf("@%d", index+5),
			"M=D",
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func function(name string, nVars string) (string, error) {
	numVars, err := strconv.Atoi(nVars)
	if err != nil {
		return "", fmt.Errorf("invalid vars for function definition (%s): %s", name, nVars)
	}

	// Change the function context
	funcStack.current = name

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

	return strings.Join(lines, "\n") + "\n", nil
}

func callFunction(name string, nArgs string) (string, error) {
	numArgs, err := strconv.Atoi(nArgs)
	if err != nil {
		return "", fmt.Errorf("invalid args to function (%s): %s", name, nArgs)
	}

	callingFuncName := funcStack.current
	returnLabel := getFolderName() + "." + callingFuncName + "$ret" + strconv.Itoa(funcStack.returnCounter)

	lines := []string{
		// Put the function address into the `locRegister`
		fmt.Sprintf("@%s", getFolderName()+"."+name),
		"D=A",
		locRegister,
		"M=D",

		// Put the number of args into the `valueRegister`
		fmt.Sprintf("@%d", numArgs),
		"D=A",
		valueRegister,
		"M=D",

		// Put the return address into the D register
		fmt.Sprintf("@%s", returnLabel),
		"D=A",

		// Jump to the call routine
		"@CALL",
		"0;JMP",

		// Set the return label for this call
		fmt.Sprintf("(%s)", returnLabel),
	}

	// Increment the return counter for the next call from this function
	funcStack.returnCounter++

	return strings.Join(lines, "\n") + "\n", nil
}

func returnFromFunction() string {
	lines := []string{
		"@RETURN",
		"0;JMP",
	}

	return strings.Join(lines, "\n") + "\n"
}

func gotoLabel(label string) string {
	callingFuncName := funcStack.current
	constructedLabel := callingFuncName + "$" + label

	lines := []string{
		fmt.Sprintf("@%s", constructedLabel),
		"0;JMP",
	}

	return strings.Join(lines, "\n") + "\n"
}

func ifGoto(label string) string {
	callingFuncName := funcStack.current
	constructedLabel := callingFuncName + "$" + label

	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		fmt.Sprintf("@%s", constructedLabel),
		"D;JNE",
	}

	return strings.Join(lines, "\n") + "\n"
}

func label(label string) string {
	callingFuncName := funcStack.current
	constructedLabel := callingFuncName + "$" + label

	return fmt.Sprintf("(%s)", constructedLabel) + "\n"
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

func add() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"A=A-1",
		"M=D+M",
	}

	return strings.Join(lines, "\n")
}

func sub() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"A=A-1",
		"M=M-D",
	}

	return strings.Join(lines, "\n")
}

func neg() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"M=-M",
		"@SP",
		"M=M+1",
	}

	return strings.Join(lines, "\n")
}

var eqCount = 0

func eq() string {
	retAddress := fmt.Sprintf("RET_ADDRESS_EQ%d", eqCount)

	lines := []string{
		fmt.Sprintf("@%s", retAddress),
		"D=A",
		"@EQ",
		"0;JMP",
		fmt.Sprintf("(%s)", retAddress),
	}

	eqCount++

	return strings.Join(lines, "\n")
}

var gtCount = 0

func gt() string {
	retAddress := fmt.Sprintf("RET_ADDRESS_GT%d", gtCount)

	lines := []string{
		fmt.Sprintf("@%s", retAddress),
		"D=A",
		"@GT",
		"0;JMP",
		fmt.Sprintf("(%s)", retAddress),
	}

	gtCount++

	return strings.Join(lines, "\n")
}

var ltCount = 0

func lt() string {

	retAddress := fmt.Sprintf("RET_ADDRESS_LT%d", ltCount)

	lines := []string{
		fmt.Sprintf("@%s", retAddress),
		"D=A",
		"@LT",
		"0;JMP",
		fmt.Sprintf("(%s)", retAddress),
	}

	ltCount++

	return strings.Join(lines, "\n")
}

func and() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"A=A-1",
		"M=D&M",
	}

	return strings.Join(lines, "\n")
}

func or() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"D=M",
		"A=A-1",
		"M=D|M",
	}

	return strings.Join(lines, "\n")
}

func not() string {
	lines := []string{
		"@SP",
		"AM=M-1",
		"M=!M",
		"@SP",
		"M=M+1",
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

func getFolderName() string {
	// Get the name of the current folder
	dir := pathToTranslate

	return path.Base(dir)
}

// func (s *Stack) Push(item string) {
// 	s.items = append(s.items, item)
// }

// func (s *Stack) Pop() {
// 	s.items = s.items[:len(s.items)-1]
// }

// func (s *Stack) Peek() string {
// 	return s.items[len(s.items)-1]
// }
