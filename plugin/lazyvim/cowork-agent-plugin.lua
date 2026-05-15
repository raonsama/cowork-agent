return {
  {
    dir = "~/.config/nvim/lua/cowork-agent",  -- or wherever you place local plugins
    name = "cowork-agent",
    lazy = false,
    config = function()
      require("cowork-agent.cowork").setup({})
    end,
  }
}
