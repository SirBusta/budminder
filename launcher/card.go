package launcher

import (
	"fmt"
	"strings"
)

// A card is the small window the Windows program shows in place of a console:
// the welcome window when BudMinder starts, and the questions and problems
// that come before it runs (an update to confirm, a port already in use, a
// database that won't open). It looks like the phone window: paper, a tan
// frame with a seedling in its top edge (red for problems), Font Awesome icons.
// The words live here, on every system, so they can be tested; drawing them is
// card_windows.go.

// SpanStyle is how a piece of text is drawn.
type SpanStyle int

const (
	Plain SpanStyle = iota
	Bold
	Code // a path or a version: fixed-width
)

// Span is a piece of text in one style.
type Span struct {
	Text  string
	Style SpanStyle
}

// Words turns text with **bold** and `code` marks into spans.
func Words(s string) []Span {
	var out []Span
	style := Plain
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{b.String(), style})
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		switch {
		case style != Code && strings.HasPrefix(s[i:], "**"):
			flush()
			if style == Bold {
				style = Plain
			} else {
				style = Bold
			}
			i++
		case s[i] == '`':
			flush()
			if style == Code {
				style = Plain
			} else {
				style = Code
			}
		default:
			b.WriteByte(s[i])
		}
	}
	flush()
	return out
}

func spansText(s []Span) string {
	var b strings.Builder
	for _, sp := range s {
		b.WriteString(sp.Text)
	}
	return b.String()
}

// BlockKind is one part of a card, top to bottom.
type BlockKind int

const (
	BlockBanner  BlockKind = iota // a tinted note above the title (Warn: amber, else green)
	BlockSub                      // centered words under the title
	BlockButtons                  // a row of buttons, centered
	BlockRows                     // icon + words, one per line
	BlockCheck                    // a check box with an icon in front
	BlockDivider                  // two rules with the seedling between them
	BlockHead                     // a small heading
	BlockLink                     // centered, underlined, clickable words
	BlockDetails                  // the technical message, small and fixed-width
)

// Row is one line of a BlockRows.
type Row struct {
	Icon    rune // a Font Awesome glyph
	AppIcon bool // the BudMinder icon instead
	Text    []Span
}

// Button is one button of a BlockButtons. Main is the green one.
type Button struct {
	ID    int
	Label string
	Icon  rune
	Main  bool
}

// Block is one part of a card.
type Block struct {
	Kind    BlockKind
	Text    []Span // Banner, Sub, Head, Link, Check, Details
	Icon    rune   // Banner, Check
	Warn    bool   // Banner
	Rows    []Row
	Buttons []Button
	ID      int  // Link, Check
	Checked bool // Check
}

// Card is a whole window.
type Card struct {
	Caption string // the title bar
	Title   string // big, centered
	Bad     bool   // a problem: red seedling and icons
	Blocks  []Block
}

// What the buttons, the check box and the link do (main acts on these IDs).
const (
	CardGotIt = iota + 1
	CardDesktop
	CardPhone
	CardUpdate
	CardNotNow
	CardStopOther
	CardCancel
	CardOtherPort
	CardCopy
	CardClose
	CardFolder
	CardChooseFolder
	CardHelp
)

// Font Awesome 6 solid glyphs used on cards.
const (
	faList      = ''
	faDesktop   = ''
	faFolder    = '' // folder-open
	faChevronUp = ''
	faMoon      = ''
	faCheck     = ''
	faCircleOK  = '' // circle-check
	faWarn      = '' // triangle-exclamation
	faFloppy    = ''
	faRotate    = ''
	faDownload  = ''
	faPower     = ''
	faShuffle   = ''
	faClockBack = '' // clock-rotate-left
	faComments  = ''
	faCopy      = ''
	faLock      = ''
)

// WelcomeCard is the welcome window: what's running, where it opened, the
// icons, where the data lives and, with the tray icon, where BudMinder went.
// w.Folder is the program's folder; version is this BudMinder's.
func WelcomeCard(w Welcome, version string) Card {
	c := Card{Caption: "BudMinder", Title: "BudMinder is running"}
	add := func(b Block) { c.Blocks = append(c.Blocks, b) }
	banner := func(warn bool, icon rune, s string) {
		add(Block{Kind: BlockBanner, Warn: warn, Icon: icon, Text: Words(s)})
	}
	switch w.Risky {
	case "Downloads":
		banner(true, faWarn, "BudMinder is running from your **Downloads** folder and keeps your data there, where clean-ups can delete it. "+
			"Stop it, move the program into its own folder (for example `"+w.Suggest+"`) and start it from there.")
	case "":
	default: // a syncing app (Synced)
		banner(true, faWarn, "BudMinder is running from a **"+w.Risky+"** folder. "+w.Risky+" copies files to the cloud while BudMinder is still writing to them, which can damage your grow journal. "+
			"Stop BudMinder, move the program into its own folder (for example `"+w.Suggest+"`) and start it from there. "+
			"To keep a copy in the cloud, put BudMinder's backups there instead.")
	}
	if w.Installed != "" {
		banner(false, faCircleOK, "BudMinder set itself up in `"+w.Folder+"`. You can delete the file you downloaded.")
	}
	if w.Updated != "" {
		c.Title = "BudMinder is up to date"
		banner(false, faCircleOK, "Updated to **"+version+"**. A copy of your database from before is in `data\\backups`. You can delete the file you downloaded.")
	}
	if w.Elsewhere != "" {
		banner(true, faWarn, "This is a second copy of BudMinder, with its own data. Your BudMinder icons open the one in `"+w.Elsewhere+"`.")
	}

	sub := "BudMinder opened in your default browser."
	if w.Setup {
		sub += " Head there to set up your username and password."
	}
	add(Block{Kind: BlockSub, Text: Words(sub)})

	switch {
	case w.IconsMade:
		add(Block{Kind: BlockRows, Rows: []Row{{Icon: faList, Text: Words("BudMinder is available in your Start menu alongside **Stop BudMinder**.")}}})
		add(Block{Kind: BlockCheck, Icon: faDesktop, ID: CardDesktop, Checked: w.IconDesktop, Text: Words("Put a BudMinder icon on the desktop")})
		add(Block{Kind: BlockRows, Rows: []Row{{Icon: faFolder, Text: Words("Your data and journal files are stored in `" + w.Folder + "`. Do not delete this folder, as it contains your database and photos.")}}})
	case w.HasIcon && w.Installed == "" && w.Updated == "":
		add(Block{Kind: BlockRows, Rows: []Row{{Icon: faList, Text: Words("Next time, start BudMinder from the Start menu instead of this file.")}}})
	}

	if w.Tray {
		add(Block{Kind: BlockDivider})
		add(Block{Kind: BlockHead, Text: Words("BudMinder runs in the background")})
		add(Block{Kind: BlockRows, Rows: []Row{
			{AppIcon: true, Text: Words("Look for the green leaf icon in the system tray (next to the clock). Click it to open BudMinder, or right-click it for quick options, including **Stop BudMinder** and opening BudMinder on your phone.")},
			{Icon: faChevronUp, Text: Words("Hidden icon? Click the arrow (**^**) in the system tray and drag the BudMinder leaf onto the taskbar so it always stays visible.")},
			{Icon: faMoon, Text: Words("Closing this window or your browser does not shut down BudMinder. You can still connect to BudMinder with your phone, and your grow sensor data keeps being logged without interruption. To stop it, use **Stop BudMinder** in the Start menu, or right-click the tray icon.")},
		}})
	} else {
		add(Block{Kind: BlockRows, Rows: []Row{{Icon: faMoon, Text: Words("Closing this window or your browser does not shut down BudMinder. To stop it, use **Stop BudMinder** in the Start menu.")}}})
	}
	add(Block{Kind: BlockButtons, Buttons: []Button{{ID: CardGotIt, Label: "Got it", Icon: faCheck}}})
	if w.Tray && w.LANAddress != "" {
		add(Block{Kind: BlockLink, ID: CardPhone, Text: Words("Open BudMinder on your phone")})
	}
	return c
}

// TrayHint is the Windows notification after the welcome window goes to the
// tray. hidden: Windows keeps the icon behind the ^.
func TrayHint(hidden bool) (title, text string) {
	if hidden {
		return "BudMinder is still running", "Windows put its leaf icon behind the ^ by the clock. Click the ^ to find it, and drag it onto the taskbar to keep it in view."
	}
	return "BudMinder is still running", "It's the leaf icon by the clock. Click it to open BudMinder, or right-click it for more, including stopping BudMinder."
}

// UpdateCard asks whether a double-clicked download should update the
// install in dir.
func UpdateCard(dir, from, to string) Card {
	return Card{Caption: "BudMinder", Title: "Update BudMinder?", Blocks: []Block{
		{Kind: BlockSub, Text: Words(fmt.Sprintf("BudMinder **%s** is installed in `%s`. This download is **%s**.", from, dir, to))},
		{Kind: BlockRows, Rows: []Row{
			{Icon: faFloppy, Text: Words("Your database is backed up first, and your data stays where it is.")},
			{Icon: faRotate, Text: Words("BudMinder stops for a moment, updates and starts again.")},
		}},
		{Kind: BlockButtons, Buttons: []Button{{ID: CardUpdate, Label: "Update", Icon: faDownload, Main: true}, {ID: CardNotNow, Label: "Not now"}}},
	}}
}

// UpdateFailedCard says the update didn't happen and nothing changed.
func UpdateFailedCard(dir, details string) Card {
	return Card{Caption: "BudMinder", Title: "The update didn't work", Bad: true, Blocks: []Block{
		{Kind: BlockSub, Text: Words("Your BudMinder in `" + dir + "` is unchanged. Start it as usual, and try the update again later.")},
		{Kind: BlockButtons, Buttons: []Button{{ID: CardCopy, Label: "Copy details", Icon: faCopy}, {ID: CardClose, Label: "Close"}}},
		{Kind: BlockDetails, Text: []Span{{details, Code}}},
	}}
}

// OtherRunningCard: a BudMinder from this folder is still running (usually
// the old version, after the program file was replaced), so this one can't
// start. running is its version, "" when it's too old to say.
func OtherRunningCard(running, port string, older bool) Card {
	title, who := "Another BudMinder is still running", "A BudMinder"
	if older {
		title = "An older BudMinder is still running"
	}
	if running != "" {
		who = "BudMinder **" + running + "**"
	}
	return Card{Caption: "BudMinder", Title: title, Bad: true, Blocks: []Block{
		{Kind: BlockSub, Text: Words(who + " is still running from this folder and using port " + port + ", so this one can't start.")},
		{Kind: BlockButtons, Buttons: []Button{{ID: CardStopOther, Label: "Stop it and start this one", Icon: faPower, Main: true}, {ID: CardCancel, Label: "Cancel"}}},
	}}
}

// PortTakenCard: another program uses BudMinder's port. alt is a free port
// BudMinder could use instead ("" when none was found).
func PortTakenCard(port, alt, details string) Card {
	rows := []Row{{Icon: faPower, Text: Words("Close the program that's using it, then start BudMinder again.")}}
	buttons := []Button{}
	if alt != "" {
		rows = append(rows, Row{Icon: faShuffle, Text: Words("Or let BudMinder use port **" + alt + "** instead. Your bookmarks and phone will need the new address.")})
		buttons = append(buttons, Button{ID: CardOtherPort, Label: "Use port " + alt, Icon: faShuffle, Main: true})
	}
	buttons = append(buttons, Button{ID: CardCopy, Label: "Copy details", Icon: faCopy}, Button{ID: CardClose, Label: "Close"})
	return Card{Caption: "BudMinder", Title: "BudMinder couldn't start", Bad: true, Blocks: []Block{
		{Kind: BlockSub, Text: Words("Another program is using port **" + port + "**, which BudMinder needs.")},
		{Kind: BlockRows, Rows: rows},
		{Kind: BlockButtons, Buttons: buttons},
		{Kind: BlockDetails, Text: []Span{{details, Code}}},
	}}
}

// StartFailedCard: BudMinder stopped while starting (the log's last fatal
// message is details). A message about the database gets the database help.
func StartFailedCard(details string) Card {
	stuck := Row{Icon: faComments, Text: Words("Still stuck? Click **Get help** below, then paste the details into a new issue.")}
	buttons := Block{Kind: BlockButtons, Buttons: []Button{{ID: CardFolder, Label: "Open BudMinder folder", Icon: faFolder}, {ID: CardCopy, Label: "Copy details", Icon: faCopy}, {ID: CardClose, Label: "Close"}}}
	help := Block{Kind: BlockLink, ID: CardHelp, Text: Words("Get help")}
	l := strings.ToLower(details)
	if strings.Contains(l, "database") || strings.Contains(l, "sqlite") || strings.Contains(l, "migrat") {
		return Card{Caption: "BudMinder", Title: "BudMinder couldn't open your grow journal", Bad: true, Blocks: []Block{
			{Kind: BlockSub, Text: Words("Your database, `data\\isley.db` in the BudMinder folder, couldn't be opened.")},
			{Kind: BlockRows, Rows: []Row{
				{Icon: faCloud, Text: Words("Is the BudMinder folder in OneDrive, Dropbox, Google Drive or another syncing folder? Syncing can damage the database: move BudMinder into a folder of its own.")},
				{Icon: faClockBack, Text: Words("Copies from before each update are in `data\\backups`.")},
				stuck,
			}},
			buttons,
			{Kind: BlockDetails, Text: []Span{{details, Code}}},
			help,
		}}
	}
	return Card{Caption: "BudMinder", Title: "BudMinder couldn't start", Bad: true, Blocks: []Block{
		{Kind: BlockSub, Text: Words("Something went wrong while BudMinder was starting.")},
		{Kind: BlockRows, Rows: []Row{
			{Icon: faRotate, Text: Words("Try starting it again. If it happens again, the log in `logs\\app.log` has more.")},
			stuck,
		}},
		buttons,
		{Kind: BlockDetails, Text: []Span{{details, Code}}},
		help,
	}}
}

const faCloud = '' // Font Awesome cloud

// HelpURL is where Get help on a problem window goes.
const HelpURL = "https://github.com/SirBusta/budminder/issues"

// SetupFailedCard: BudMinder couldn't copy itself into the chosen folder.
func SetupFailedCard(dir, details string) Card {
	return Card{Caption: "BudMinder", Title: "BudMinder couldn't set up there", Bad: true, Blocks: []Block{
		{Kind: BlockSub, Text: Words("BudMinder couldn't copy itself into `" + dir + "`.")},
		{Kind: BlockRows, Rows: []Row{{Icon: faFolder, Text: Words("Pick a folder in your own user folder instead, for example `" + SuggestedHome() + "`.")}}},
		{Kind: BlockButtons, Buttons: []Button{{ID: CardChooseFolder, Label: "Choose another folder", Icon: faFolder, Main: true}, {ID: CardCopy, Label: "Copy details", Icon: faCopy}, {ID: CardCancel, Label: "Cancel"}}},
		{Kind: BlockDetails, Text: []Span{{details, Code}}},
	}}
}
