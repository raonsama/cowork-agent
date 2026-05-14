-- cowork-agent.lua
-- LazyVim plugin bridge for CoworkAgent
-- Place in ~/.config/nvim/lua/plugins/cowork-agent.lua
-- or load via your lazy.nvim spec.

local M = {}

-- ── Configuration ─────────────────────────────────────────────────────────────

M.config = {
  -- Path to the compiled cowork binary
  bin = vim.fn.expand("~/.local/bin/cowork"),
  -- Ollama model override (empty = use config default)
  model = "",
  -- Float window dimensions (fraction of editor size)
  win_width_ratio  = 0.55,
  win_height_ratio = 0.75,
  -- Sidebar width (columns) when using sidebar mode
  sidebar_width = 60,
  -- Default window mode: "float" | "sidebar" | "tab"
  window_mode = "float",
  -- Keymap prefix
  prefix = "<leader>cw",
}

-- ── State ─────────────────────────────────────────────────────────────────────

local state = {
  bufnr   = nil,
  winid   = nil,
  job_id  = nil,
  mode    = nil,
}

-- ── Helpers ───────────────────────────────────────────────────────────────────

--- Returns true if CoworkAgent window is currently open.
local function is_open()
  return state.winid ~= nil and vim.api.nvim_win_is_valid(state.winid)
end

--- Returns true if CoworkAgent buffer exists and is valid.
local function buf_valid()
  return state.bufnr ~= nil and vim.api.nvim_buf_is_valid(state.bufnr)
end

--- Resolves the project root (git root → cwd fallback).
local function project_root()
  local git_root = vim.fn.systemlist("git rev-parse --show-toplevel 2>/dev/null")[1]
  if git_root and git_root ~= "" then
    return git_root
  end
  return vim.fn.getcwd()
end

--- Creates or recycles the CoworkAgent terminal buffer.
local function get_or_create_buf()
  if buf_valid() then return state.bufnr end

  local bufnr = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_option(bufnr, "bufhidden", "hide")
  vim.api.nvim_buf_set_option(bufnr, "filetype",  "cowork-agent")
  vim.api.nvim_buf_set_name(bufnr, "CoworkAgent")
  state.bufnr = bufnr
  return bufnr
end

-- ── Window modes ──────────────────────────────────────────────────────────────

--- Opens a centred floating window.
local function open_float(bufnr)
  local cfg   = M.config
  local ui    = vim.api.nvim_list_uis()[1]
  local width  = math.floor(ui.width  * cfg.win_width_ratio)
  local height = math.floor(ui.height * cfg.win_height_ratio)
  local row    = math.floor((ui.height - height) / 2)
  local col    = math.floor((ui.width  - width)  / 2)

  local winid = vim.api.nvim_open_win(bufnr, true, {
    relative = "editor",
    width    = width,
    height   = height,
    row      = row,
    col      = col,
    style    = "minimal",
    border   = "rounded",
    title    = " 🤖 CoworkAgent ",
    title_pos = "center",
  })

  -- Float-specific window options
  vim.api.nvim_win_set_option(winid, "winblend",   8)
  vim.api.nvim_win_set_option(winid, "cursorline", true)
  return winid
end

--- Opens a vertical sidebar on the right.
local function open_sidebar(bufnr)
  vim.cmd("botright vsplit")
  local winid = vim.api.nvim_get_current_win()
  vim.api.nvim_win_set_width(winid, M.config.sidebar_width)
  vim.api.nvim_win_set_buf(winid, bufnr)
  vim.api.nvim_win_set_option(winid, "winfixwidth", true)
  return winid
end

--- Opens in a new tab.
local function open_tab(bufnr)
  vim.cmd("tabnew")
  local winid = vim.api.nvim_get_current_win()
  vim.api.nvim_win_set_buf(winid, bufnr)
  return winid
end

--- Opens a window using the configured mode.
local function open_window(bufnr, mode)
  mode = mode or M.config.window_mode
  state.mode = mode
  if mode == "sidebar" then return open_sidebar(bufnr) end
  if mode == "tab"     then return open_tab(bufnr) end
  return open_float(bufnr)
end

-- ── Terminal launch ───────────────────────────────────────────────────────────

--- Builds the cowork CLI command list.
local function build_cmd(task)
  local cfg  = M.config
  local root = project_root()
  local args = { cfg.bin, "--project", root }
  if cfg.model ~= "" then
    vim.list_extend(args, { "--model", cfg.model })
  end
  if task and task ~= "" then
    vim.list_extend(args, { "cowork", task })
  end
  return args
end

--- Starts the CoworkAgent terminal in the given buffer.
local function start_terminal(bufnr, task)
  local cmd = build_cmd(task)
  state.job_id = vim.fn.termopen(cmd, {
    cwd = project_root(),
    on_exit = function(_, code)
      state.job_id = nil
      if code ~= 0 then
        vim.schedule(function()
          vim.notify(
            string.format("CoworkAgent exited with code %d", code),
            vim.log.levels.WARN,
            { title = "CoworkAgent" }
          )
        end)
      end
    end,
  })
  -- Enter terminal insert mode automatically
  vim.cmd("startinsert")
end

-- ── Public API ────────────────────────────────────────────────────────────────

--- Toggle the CoworkAgent window (open / close).
function M.toggle(mode)
  if is_open() then
    vim.api.nvim_win_close(state.winid, false)
    state.winid = nil
    return
  end
  M.open(mode)
end

--- Open CoworkAgent in the specified mode.
function M.open(mode)
  if is_open() then
    vim.api.nvim_set_current_win(state.winid)
    return
  end

  local bufnr = get_or_create_buf()
  local winid = open_window(bufnr, mode)
  state.winid = winid

  -- Only launch terminal if not already running
  if state.job_id == nil then
    start_terminal(bufnr, nil)
  end
end

--- Send a cowork task directly (opens window and passes task to CLI).
function M.send_task(task)
  -- If window open and job running, just send input
  if is_open() and state.job_id ~= nil then
    vim.api.nvim_chan_send(state.job_id, task .. "\n")
    return
  end

  -- Otherwise start fresh with the task
  local bufnr = get_or_create_buf()
  -- Wipe previous terminal content
  if buf_valid() and state.job_id == nil then
    vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, {})
  end

  local winid = open_window(bufnr, M.config.window_mode)
  state.winid = winid
  start_terminal(bufnr, task)
end

--- Prompt the user for a task via vim.ui.input, then launch cowork mode.
function M.prompt_task()
  vim.ui.input({ prompt = "🤖 CoworkAgent task: " }, function(input)
    if input and input ~= "" then
      M.send_task(input)
    end
  end)
end

--- Send the current visual selection as context to the agent.
function M.send_selection()
  local start_line = vim.fn.getpos("'<")[2]
  local end_line   = vim.fn.getpos("'>")[2]
  local lines      = vim.api.nvim_buf_get_lines(0, start_line - 1, end_line, false)
  local text       = table.concat(lines, "\n")
  local filename   = vim.fn.expand("%:t")

  local prompt = string.format(
    "Context from %s (lines %d–%d):\n```\n%s\n```\n",
    filename, start_line, end_line, text
  )

  if is_open() and state.job_id ~= nil then
    vim.api.nvim_chan_send(state.job_id, prompt)
  else
    vim.notify("CoworkAgent is not running. Use <leader>cwo to open it first.",
      vim.log.levels.INFO, { title = "CoworkAgent" })
  end
end

--- Index the current project.
function M.index()
  local root = project_root()
  local bufnr = get_or_create_buf()
  local winid = open_window(bufnr, M.config.window_mode)
  state.winid = winid
  local cmd = { M.config.bin, "index", root }
  state.job_id = vim.fn.termopen(cmd, { cwd = root })
  vim.cmd("startinsert")
end

-- ── Keymaps ───────────────────────────────────────────────────────────────────

function M.setup_keymaps()
  local p = M.config.prefix
  local opts = { noremap = true, silent = true }

  -- Toggle float
  vim.keymap.set("n", p .. "t", M.toggle,        vim.tbl_extend("force", opts, { desc = "CoworkAgent: Toggle" }))
  -- Open in specific modes
  vim.keymap.set("n", p .. "o", function() M.open("float")   end, vim.tbl_extend("force", opts, { desc = "CoworkAgent: Open float" }))
  vim.keymap.set("n", p .. "s", function() M.open("sidebar") end, vim.tbl_extend("force", opts, { desc = "CoworkAgent: Open sidebar" }))
  vim.keymap.set("n", p .. "T", function() M.open("tab")     end, vim.tbl_extend("force", opts, { desc = "CoworkAgent: Open tab" }))
  -- Send task
  vim.keymap.set("n", p .. "p", M.prompt_task,   vim.tbl_extend("force", opts, { desc = "CoworkAgent: Prompt task" }))
  -- Send visual selection as context
  vim.keymap.set("v", p .. "c", M.send_selection, vim.tbl_extend("force", opts, { desc = "CoworkAgent: Send selection" }))
  -- Index project
  vim.keymap.set("n", p .. "i", M.index,          vim.tbl_extend("force", opts, { desc = "CoworkAgent: Index project" }))
end

-- ── Autocmds ──────────────────────────────────────────────────────────────────

local function setup_autocmds()
  local group = vim.api.nvim_create_augroup("CoworkAgent", { clear = true })

  -- Close float when focus leaves
  vim.api.nvim_create_autocmd("WinLeave", {
    group = group,
    callback = function()
      if state.mode == "float" and is_open() and
         vim.api.nvim_get_current_win() ~= state.winid then
        -- Don't auto-close; let the user toggle
      end
    end,
  })

  -- Cleanup on VimLeave
  vim.api.nvim_create_autocmd("VimLeave", {
    group = group,
    callback = function()
      if state.job_id then
        vim.fn.jobstop(state.job_id)
      end
    end,
  })
end

-- ── Plugin entry point ────────────────────────────────────────────────────────

--- Call M.setup() in your lazy.nvim spec config field.
function M.setup(opts)
  M.config = vim.tbl_deep_extend("force", M.config, opts or {})
  M.setup_keymaps()
  setup_autocmds()

  -- Register user commands
  vim.api.nvim_create_user_command("CoworkToggle",  function() M.toggle() end,       { desc = "Toggle CoworkAgent" })
  vim.api.nvim_create_user_command("CoworkTask",    function(o) M.send_task(o.args) end, { nargs = "+", desc = "Run cowork task" })
  vim.api.nvim_create_user_command("CoworkIndex",   function() M.index() end,        { desc = "Index project" })
  vim.api.nvim_create_user_command("CoworkSidebar", function() M.open("sidebar") end, { desc = "Open in sidebar" })
end

return M

--[[
── Quick Start for lazy.nvim ────────────────────────────────────────────────────

  {
    dir = "~/.config/nvim/lua/plugins",  -- or wherever you place local plugins
    name = "cowork-agent",
    lazy = false,
    config = function()
      require("plugins.cowork-agent").setup({
        bin          = vim.fn.expand("~/.local/bin/cowork"),
        window_mode  = "float",    -- "float" | "sidebar" | "tab"
        sidebar_width = 65,
        prefix       = "<leader>cw",
      })
    end,
  }

── Keymaps ──────────────────────────────────────────────────────────────────────

  <leader>cwt  — Toggle CoworkAgent window
  <leader>cwo  — Open as float
  <leader>cws  — Open as sidebar
  <leader>cwT  — Open in new tab
  <leader>cwp  — Prompt for a new task
  <leader>cwc  — Send visual selection as context (visual mode)
  <leader>cwi  — Index current project

  :CoworkToggle
  :CoworkTask <description>
  :CoworkIndex
  :CoworkSidebar

──────────────────────────────────────────────────────────────────────────────]]
