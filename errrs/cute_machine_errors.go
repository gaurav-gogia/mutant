package errrs

import (
	"math/rand"
	"sync"
	"time"
)

var machineComfortCycler = newMessageCycler(
	[]string{
		"The VM tripped over a tiny semicolon and is trying again with confidence.",
		"A gentle runtime wobble happened. Your code buddy is still cheering for you.",
		"Tiny bytecode gremlins caused a hiccup. Nothing personal, just one of those days.",
		"The runtime bumped into a snack break. Let's fix this little bump together.",
		"Your program found a spicy edge case. The VM brought a cozy blanket.",
		"Oopsie in motion: the VM is polishing its gears and pointing at the exact issue below.",
		"A small runtime boop occurred. You are still doing great.",
		"The VM did a dramatic faint, then left a useful clue right below.",
		"A curious opcode took a wrong turn. Good news: we caught it safely.",
		"Runtime turbulence detected. Friendly diagnostics are now serving warm tea.",
	},
	rand.New(rand.NewSource(time.Now().UnixNano())),
)

var parserComfortCycler = newMessageCycler(
	[]string{
		"The parser squinted at that line, then politely asked for a tiny rewrite.",
		"Syntax sprites got tangled for a second; the clues below will untie them.",
		"Your code is creative. The parser just needs a little punctuation hug.",
		"A grammar hiccup happened, and the parser left breadcrumbs for you below.",
		"Parse puzzle detected. Totally fixable, and you're very close.",
		"The parser tripped on a comma pebble; diagnostic treasure is right below.",
		"Tiny syntax wobble. Big progress energy still intact.",
		"Your idea is solid; the parser just wants the tokens in a cozier order.",
	},
	rand.New(rand.NewSource(time.Now().UnixNano()+17)),
)

var compilerComfortCycler = newMessageCycler(
	[]string{
		"The compiler is knitting your program and dropped one stitch. Let's patch it.",
		"A bytecode butterfly flapped the wrong way; details are right below.",
		"Compilation took a tiny detour. The map back is in this error message.",
		"The compiler found a spicy construct and asked for one small adjustment.",
		"Bits are friendly but picky today. You'll get this fixed quickly.",
		"A codegen gremlin made a tiny squeak; diagnostics are waiting below.",
		"Build wobble detected. Your logic is still doing great.",
		"The compiler paused for tea and left exact notes for the next step.",
	},
	rand.New(rand.NewSource(time.Now().UnixNano()+31)),
)

type messageCycler struct {
	mu       sync.Mutex
	messages []string
	order    []string
	index    int
	last     string
	hasLast  bool
	rng      *rand.Rand
}

func newMessageCycler(messages []string, rng *rand.Rand) *messageCycler {
	cloned := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == "" {
			continue
		}
		cloned = append(cloned, message)
	}

	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	cycler := &messageCycler{
		messages: cloned,
		rng:      rng,
	}
	cycler.refreshOrderLocked()
	return cycler
}

func (c *messageCycler) next() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.messages) == 0 {
		return "Even machines aren't perfect. Below error messages may help!"
	}

	if c.index >= len(c.order) {
		c.refreshOrderLocked()
	}

	message := c.order[c.index]
	c.index++
	c.last = message
	c.hasLast = true
	return message
}

func (c *messageCycler) refreshOrderLocked() {
	c.order = append(c.order[:0], c.messages...)
	for i := len(c.order) - 1; i > 0; i-- {
		j := c.rng.Intn(i + 1)
		c.order[i], c.order[j] = c.order[j], c.order[i]
	}

	if c.hasLast && len(c.order) > 1 && c.order[0] == c.last {
		c.order[0], c.order[1] = c.order[1], c.order[0]
	}

	c.index = 0
}

func nextMachineComfortMessage() string {
	return machineComfortCycler.next()
}

func nextParserComfortMessage() string {
	return parserComfortCycler.next()
}

func nextCompilerComfortMessage() string {
	return compilerComfortCycler.next()
}
