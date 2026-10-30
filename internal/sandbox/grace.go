package sandbox

import "time"

// killGrace is how long a command gets between SIGTERM and SIGKILL when its context ends.
var killGrace = 3 * time.Second
