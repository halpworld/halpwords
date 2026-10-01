package game

// TestScenes gives a Context a scene stack starting with first, for tests
// of scenes in other packages. next carries out a Replace or Push the
// scene asked for and returns the scene on top then, or nil when it asked
// for nothing.
func (c *Context) TestScenes(first Scene) (next func() Scene) {
	m := &manager{stack: []Scene{first}}
	c.scenes = m
	return func() Scene {
		if m.pending == nil {
			return nil
		}
		m.pending()
		m.pending = nil
		return m.top()
	}
}

// TestOpen opens the learners, the link and the profile as a start-up
// does, in the save folder of the test.
func (c *Context) TestOpen() {
	c.openLearners()
	c.openLink()
	c.loadProfile()
}
