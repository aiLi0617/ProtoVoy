import * as React from "react";
import type { LucideProps } from "lucide-react";
import {
  Activity,
  Archive,
  ArrowDown,
  ArrowUp,
  Bolt,
  Boxes,
  Brush as BroomIcon,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Code2,
  Command,
  Copy,
  Database,
  Download,
  ExternalLink,
  Folder,
  GitBranch,
  Globe2,
  Menu,
  Moon,
  MoreHorizontal,
  Network,
  Pencil,
  Plus,
  RadioTower,
  Search,
  Send,
  Settings,
  SlidersHorizontal,
  Star,
  Sun,
  Tag,
  Triangle,
  Trash2,
  Users,
  WrapText,
  X,
} from "lucide-react";
import type { ComponentType } from "react";

void React;

export type IconName =
  | "activity"
  | "archive"
  | "arrow-down"
  | "arrow-up"
  | "bolt"
  | "broom"
  | "chevron"
  | "collapse-all"
  | "close"
  | "code"
  | "command"
  | "copy"
  | "database"
  | "download"
  | "edit"
  | "external"
  | "expand-all"
  | "folder"
  | "gitBranch"
  | "globe"
  | "menu"
  | "module"
  | "more"
  | "moon"
  | "network"
  | "plus"
  | "search"
  | "send"
  | "settings"
  | "sliders"
  | "star"
  | "sun"
  | "tag"
  | "triangle"
  | "trash"
  | "users"
  | "websocket"
  | "wrap";

const map: Record<IconName, ComponentType<LucideProps>> = {
  activity: Activity,
  archive: Archive,
  "arrow-down": ArrowDown,
  "arrow-up": ArrowUp,
  bolt: Bolt,
  broom: BroomIcon,
  chevron: ChevronRight,
  "collapse-all": ChevronsDownUp,
  close: X,
  code: Code2,
  command: Command,
  copy: Copy,
  database: Database,
  download: Download,
  edit: Pencil,
  external: ExternalLink,
  "expand-all": ChevronsUpDown,
  folder: Folder,
  gitBranch: GitBranch,
  globe: Globe2,
  menu: Menu,
  module: Boxes,
  more: MoreHorizontal,
  moon: Moon,
  network: Network,
  plus: Plus,
  search: Search,
  send: Send,
  settings: Settings,
  sliders: SlidersHorizontal,
  star: Star,
  sun: Sun,
  tag: Tag,
  triangle: Triangle,
  trash: Trash2,
  users: Users,
  websocket: RadioTower,
  wrap: WrapText,
};

export function Icon({
  name,
  size = 18,
  strokeWidth = 1.8,
  ...props
}: { name: IconName } & LucideProps) {
  const Comp = map[name];
  return <Comp size={size} strokeWidth={strokeWidth} aria-hidden="true" {...props} />;
}

/** Stable protocol / workbench icon hints (UIA-003). */
export const workbenchIconName: Record<string, IconName> = {
  http: "globe",
  graphql: "code",
  grpc: "network",
  rpc: "network",
  websocket: "websocket",
  sse: "activity",
  tcp: "network",
  udp: "network",
  mqtt: "send",
  amqp: "send",
  kafka: "activity",
  redis: "database",
  sql: "database",
  mock: "archive",
  runner: "bolt",
  gateway: "network",
  capture: "search",
  plugins: "bolt",
  ai: "star",
};
