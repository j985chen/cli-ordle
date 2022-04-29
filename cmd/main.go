package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/j985chen/cli-ordle/words"
)

const colourGreen = "\033[42m %s \033[0m"
const colourYellow = "\033[43m %s \033[0m"
const colourOrange = "\033[48;5;202m %s \033[0m"
const colourBlue = "\033[46m %s \033[0m"

var db *bolt.DB

type Player struct {
	Played        float64    `json:"played"`
	Won           float64    `json:"won"`
	CurrStreak    float64    `json:"currStreak"`
	LongestStreak float64    `json:"longestStreak"`
	Distribution  [6]float64 `json:"stats"`
	HiContrast    bool       `json:"hiContrast"`
	HardMode      bool       `json:"hardMode"`
}

func (p *Player) CreateGame() error {
	answer, err := words.RandomWord()
	if err != nil {
		return err
	}
	currGame := Game{p, []string{}, answer, false}
	err = currGame.PlayGame()
	return err
}

func (p *Player) ManageSettings(hiContrast bool, hardMode bool) error {
	p.HiContrast = hiContrast
	p.HardMode = false
	fmt.Println("---   CURRENT SETTINGS   ---")
	fmt.Printf("High-contrast\t|\t%t\nHard mode\t|\t%t\n", p.HiContrast, p.HardMode)
	return p.SaveStats()
}

func (p *Player) UpdateStatsW(numGuesses int) error {
	p.CurrStreak++
	p.LongestStreak = math.Max(p.CurrStreak, p.LongestStreak)
	p.Distribution[numGuesses-1]++
	p.Won++
	p.Played++
	return p.SaveStats()
}

func (p *Player) UpdateStatsL() error {
	p.CurrStreak = 0
	p.Played++
	return p.SaveStats()
}

func (p *Player) ViewStats() error {
	var winPercent float64
	if p.Played == 0 {
		winPercent = 0
	} else {
		winPercent = (p.Won / p.Played) * 100
	}
	fmt.Println("---     STATISTICS     ---")
	fmt.Printf("Played: %.0f | Win%%: %.0f%% | Current streak: %.0f | Longest streak: %.0f\n", p.Played, winPercent, p.CurrStreak, p.LongestStreak)
	fmt.Println()
	fmt.Println("--- GUESS DISTRIBUTION ---")
	for i := 0; i < 6; i++ {
		fmt.Printf("%d\t|\t%.0f\n", i+1, p.Distribution[i])
	}
	return nil
}

func (p *Player) SaveStats() error {
	playerBytes, err := json.Marshal(*p)
	if err != nil {
		return fmt.Errorf("could not marshal player data json: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		err = tx.Bucket([]byte("DB")).Put([]byte("PLAYER"), playerBytes)
		if err != nil {
			return fmt.Errorf("could not set player data: %v", err)
		}
		return nil
	})
	return err
}

type Game struct {
	Player       *Player
	WordsGuessed []string
	Answer       string
	Solved       bool
}

func (g *Game) ProcessGuess(guess string) error {
	isValid := words.IsValidGuess(guess)
	if !isValid {
		return fmt.Errorf("invalid")
	}
	g.WordsGuessed = append(g.WordsGuessed, guess)
	if guess == g.Answer {
		g.Solved = true
	}
	return nil
}

func (g *Game) PrintBoard() error {
	var placedColour string
	var includesColour string
	if g.Player.HiContrast {
		placedColour = colourOrange
		includesColour = colourBlue
	} else {
		placedColour = colourGreen
		includesColour = colourYellow
	}
	fmt.Printf(" ___  ___  ___  ___  ___\n")
	for i := 0; i < len(g.WordsGuessed); i++ {
		for j := 0; j < 5; j++ {
			letter := string(g.WordsGuessed[i][j])
			actual := string(g.Answer[j])

			fmt.Printf("|")
			if letter == actual {
				fmt.Printf(string(placedColour), letter)
			} else if strings.Contains(g.Answer, letter) {
				fmt.Printf(string(includesColour), letter)
			} else {
				fmt.Printf(" %s ", letter)
			}
			fmt.Printf("|")
		}
		fmt.Println("\n ---  ---  ---  ---  ---")
	}
	for i := len(g.WordsGuessed); i < 6; i++ {
		for j := 0; j < 5; j++ {
			fmt.Printf("|   |")
		}
		fmt.Println("\n ---  ---  ---  ---  ---")
	}
	fmt.Println()
	return nil
}

func (g *Game) HandleResults() error {
	if g.Solved {
		numGuesses := len(g.WordsGuessed)
		fmt.Printf("Impressive! You got the word in %d guesses\n", numGuesses)
		return g.Player.UpdateStatsW(numGuesses)

	} else {
		fmt.Printf("The answer was %s\n", g.Answer)
		return g.Player.UpdateStatsL()
	}
}

func (g *Game) PlayGame() error {
	var err error
	var input string
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("--- START OF CLIORDLE GAME ---\n")
	for i := 1; i <= 6; i++ {
		wordErr := fmt.Errorf("invalid")
		for wordErr != nil {
			fmt.Printf("Guess %d/6: ", i)
			input, err = reader.ReadString('\n')
			guess := strings.ToLower(strings.TrimSuffix(input, "\n"))
			wordErr = g.ProcessGuess(guess)
			if wordErr != nil {
				fmt.Printf("%s is an invalid guess, try again\n", guess)
			}
		}
		g.PrintBoard()
		if g.Solved {
			break
		}
	}
	g.HandleResults()
	return err
}

func setupDB() error {
	var dbErr error
	db, dbErr = bolt.Open("cliordle.db", 0600, nil)

	if dbErr != nil {
		return fmt.Errorf("could not open db, %v", dbErr)
	}

	dbErr = db.Update(func(tx *bolt.Tx) error {
		_, bucketErr := tx.CreateBucketIfNotExists([]byte("DB"))
		if bucketErr != nil {
			return fmt.Errorf("could not create root bucket: %v", bucketErr)
		}
		return nil
	})
	if dbErr != nil {
		return fmt.Errorf("could not set up buckets, %v", dbErr)
	}
	return nil
}

func initPlayer() (Player, error) {
	var player Player
	err := db.View(func(tx *bolt.Tx) error {
		playerBytes := tx.Bucket([]byte("DB")).Get([]byte("PLAYER"))
		var dbErr error = nil
		if playerBytes != nil {
			dbErr = json.Unmarshal(playerBytes, &player)
		} else {
			player = Player{0, 0, 0, 0, [6]float64{0}, false, false}
		}
		return dbErr
	})
	return player, err
}

func exitGracefully(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func manageCommands(player *Player) error {
	// cliordle subcommands
	playCommand := flag.NewFlagSet("play", flag.ExitOnError)
	settingsCommand := flag.NewFlagSet("settings", flag.ExitOnError)
	statsCommand := flag.NewFlagSet("stats", flag.ExitOnError)

	// settings command flag pointers
	settingsContrastPtr := settingsCommand.Bool("highContrast", player.HiContrast, "Turn high-contrast mode on/off")
	settingsHardModePtr := settingsCommand.Bool("hardMode", player.HardMode, "Turn hard mode on/off")

	// validate that correct number of arguments is being received
	if len(os.Args) < 2 {
		return fmt.Errorf("play, settings, or stats subcommand required")
	}

	switch os.Args[1] {
	case "play":
		playCommand.Parse(os.Args[2:])
	case "settings":
		settingsCommand.Parse(os.Args[2:])
	case "stats":
		statsCommand.Parse(os.Args[2:])
	default:
		return fmt.Errorf("play, settings, or stats subcommand required")
	}

	var err error
	if playCommand.Parsed() {
		err = player.CreateGame()
		if err != nil {
			return err
		}
	} else if settingsCommand.Parsed() {
		err = player.ManageSettings(*settingsContrastPtr, *settingsHardModePtr)
		if err != nil {
			return err
		}
	} else {
		err = player.ViewStats()
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	dbErr := setupDB()

	if dbErr != nil {
		exitGracefully(dbErr)
	}

	defer db.Close()

	// display usage info when user enters --help option
	flag.Usage = func() {
		fmt.Printf("Usage: %s [options] \nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	player, err := initPlayer()

	// processing user command
	err = manageCommands(&player)

	if err != nil {
		exitGracefully(err)
	}
}
