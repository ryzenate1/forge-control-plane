package installer

import (
	_ "gamepanel/beacon/internal/installer/operations/copyfile"
	_ "gamepanel/beacon/internal/installer/operations/downloadfile"
	_ "gamepanel/beacon/internal/installer/operations/downloadextract"
	_ "gamepanel/beacon/internal/installer/operations/fabricdl"
	_ "gamepanel/beacon/internal/installer/operations/forgedl"
	_ "gamepanel/beacon/internal/installer/operations/movefile"
	_ "gamepanel/beacon/internal/installer/operations/paperdl"
	_ "gamepanel/beacon/internal/installer/operations/removefile"
	_ "gamepanel/beacon/internal/installer/operations/runcommand"
	_ "gamepanel/beacon/internal/installer/operations/symlink"
	_ "gamepanel/beacon/internal/installer/operations/writefile"
)
