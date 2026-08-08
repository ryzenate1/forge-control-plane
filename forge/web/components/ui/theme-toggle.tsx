"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/components/theme-provider";
import { Button } from "@/components/ui/primitives";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const next = theme === "dark" ? "light" : "dark";
  return <Button aria-label={`Use ${next} theme`} onClick={toggle} size="sm" title={`Use ${next} theme`} variant="ghost">{theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}<span className="sr-only sm:not-sr-only sm:inline">{next === "light" ? "Light" : "Dark"}</span></Button>;
}
