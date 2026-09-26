//go:build js

package link

// webQueueCap is the queue's cap in a web browser, whose local storage
// holds about 5 MB for the whole game.
const webQueueCap = 10_000

func init() { defaultCap = webQueueCap }
