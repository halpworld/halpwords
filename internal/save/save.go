// Package save keeps the game's files, such as the saved adventure. On the
// desktop they are files in the user's config folder; in a web browser they
// go in the page's local storage. Files are small, so they are always read
// and written whole, and Read fails with an error matching fs.ErrNotExist
// when there is no such file. Names use forward slashes, such as
// "words/animals.txt", on every system.
package save
