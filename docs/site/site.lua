local stringify = pandoc.utils.stringify

local function read_file(p)
  local f = io.open(p, "rb")
  if not f then
    return nil
  end
  local data = f:read("a")
  f:close()
  return data
end

local function is_dir(p)
  return pcall(pandoc.system.list_directory, p)
end

local function resolve(dir, target)
  local parts = {}
  local whole = dir == "" and target or dir .. "/" .. target
  for seg in whole:gmatch("[^/]+") do
    if seg == ".." then
      if #parts == 0 then
        return nil
      end
      table.remove(parts)
    elseif seg ~= "." then
      table.insert(parts, seg)
    end
  end
  return table.concat(parts, "/")
end

local function page_of(p)
  if p == "README.md" then
    return "index.html"
  end
  local name = p:match("^docs/([^/]+)%.md$") or p:match("^docs/([^/]+)/README%.md$")
  return name and name .. ".html" or nil
end

local function out_path(p)
  return p:match("^docs/(.*)$") or p
end

local function html(s)
  return (s:gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"):gsub('"', "&quot;"))
end

local function first_header(blocks, level)
  for _, b in ipairs(blocks) do
    if b.t == "Header" and b.level == level then
      return b
    end
  end
end

local function repo_url()
  local mod = (read_file("go.mod") or ""):match("module%s+(%S+)")
  if not mod then
    error("go.mod has no module line")
  end
  return "https://" .. mod
end

local function readme_nav(readme)
  local nav, seen = {}, {}
  local blocks = readme.blocks
  for i, b in ipairs(blocks) do
    if b.t == "Header" and b.level == 2 and b.identifier == "documentation" then
      local list = blocks[i + 1]
      if not list or list.t ~= "BulletList" then
        error("README.md: no list after the Documentation heading")
      end
      for _, item in ipairs(list.content) do
        local link
        pandoc.walk_block(pandoc.Div(item), { Link = function(l) link = link or l end })
        if link then
          local p = resolve("", link.target:gsub("#.*$", ""))
          local page = p and page_of(p)
          if not page then
            error("README.md: Documentation links to " .. link.target .. ", which is not a doc page")
          end
          if not seen[page] then
            seen[page] = true
            table.insert(nav, { page = page, title = stringify(link.content) })
          end
        end
      end
    end
  end
  if #nav == 0 then
    error("README.md: no Documentation list")
  end
  local extra = {}
  for _, name in ipairs(pandoc.system.list_directory("docs")) do
    local p
    if name:match("%.md$") then
      p = "docs/" .. name
    elseif read_file("docs/" .. name .. "/README.md") and not read_file("docs/" .. name .. ".md") then
      p = "docs/" .. name .. "/README.md"
    end
    local page = p and page_of(p)
    if page and not seen[page] then
      seen[page] = true
      local h = first_header(pandoc.read(read_file(p), "gfm").blocks, 1)
      table.insert(extra, { page = page, title = h and stringify(h) or page })
    end
  end
  table.sort(extra, function(a, b) return a.title < b.title end)
  for _, e in ipairs(extra) do
    table.insert(nav, e)
  end
  return nav
end

local function pkg_doc(readme)
  local found
  readme:walk({
    Link = function(l)
      if not found and l.target:match("^https://pkg%.go%.dev/") then
        local t = stringify(l.content)
        found = { url = l.target, title = t:sub(1, 1):upper() .. t:sub(2) }
      end
    end,
  })
  return found
end

local function only_images(inlines)
  local n = 0
  for _, x in ipairs(inlines) do
    if x.t == "Image" then
      n = n + 1
    elseif x.t ~= "Space" and x.t ~= "SoftBreak" and x.t ~= "LineBreak" then
      return false
    end
  end
  return n > 0
end

local function blank(inlines)
  for _, x in ipairs(inlines) do
    if x.t ~= "Space" and x.t ~= "SoftBreak" and x.t ~= "LineBreak" then
      return false
    end
  end
  return true
end

function Pandoc(doc)
  local src = PANDOC_STATE.input_files[1]
  local here = src:match("^(.*)/[^/]*$") or ""
  local outdir = stringify(doc.meta.outdir)
  local repo = repo_url()
  local page = page_of(src)
  local readme = src == "README.md" and doc or pandoc.read(read_file("README.md"), "gfm")
  if not page then
    error(src .. " is not a doc page")
  end

  doc = doc:walk({
    Image = function(img)
      if img.src:match("^%a[%w+.-]*:") then
        return {}
      end
      local p = resolve(here, img.src)
      local data = p and read_file(p)
      if not data then
        error(src .. ": image " .. img.src .. " not found")
      end
      local dest = outdir .. "/" .. out_path(p)
      pandoc.system.make_directory(dest:match("^(.*)/[^/]*$"), true)
      local f = assert(io.open(dest, "wb"))
      f:write(data)
      f:close()
      img.src = out_path(p)
      return img
    end,
  })

  doc = doc:walk({
    Link = function(l)
      if #l.content == 0 then
        return {}
      end
      local t = l.target
      if t:match("^%a[%w+.-]*:") or t:match("^#") then
        return l
      end
      local file, frag = t:match("^([^#]*)(#?.*)$")
      local p = resolve(here, file)
      if not p then
        return l
      end
      local target = page_of(p)
      if target then
        l.target = target .. frag
      elseif read_file(p) or is_dir(p) then
        l.target = repo .. (is_dir(p) and "/tree/main/" or "/blob/main/") .. p .. frag
      end
      return l
    end,
  })

  local toc = {}
  local h1
  doc = doc:walk({
    Para = function(para)
      if blank(para.content) then
        return {}
      end
      if only_images(para.content) then
        return pandoc.Div(pandoc.Plain(para.content), { class = "figs" })
      end
    end,
    Table = function(tbl)
      return pandoc.Div({ tbl }, { class = "table" })
    end,
    Header = function(h)
      if h.level == 1 and not h1 then
        h1 = stringify(h)
      elseif h.level == 2 then
        table.insert(toc, { id = h.identifier, title = stringify(h) })
      end
    end,
  })

  local site = stringify(first_header(readme.blocks, 1) or "")
  local nav = readme_nav(readme)
  local pkg = pkg_doc(readme)

  local description
  for i, b in ipairs(doc.blocks) do
    if b.t == "Para" then
      description = stringify(b)
      if page == "index.html" then
        doc.blocks[i] = pandoc.Div({ b }, { class = "lede" })
      end
      break
    end
  end

  local items = {}
  for _, n in ipairs(nav) do
    local on = n.page == page
    local li = '<li><a href="' .. n.page .. '"' .. (on and ' class="on" aria-current="page"' or "") .. ">" .. html(n.title) .. "</a>"
    if on and #toc > 1 then
      local sub = {}
      for _, t in ipairs(toc) do
        table.insert(sub, '<li><a href="#' .. t.id .. '">' .. html(t.title) .. "</a></li>")
      end
      li = li .. '<ul class="toc">' .. table.concat(sub) .. "</ul>"
    end
    table.insert(items, li .. "</li>")
  end

  local links = {}
  if pkg then
    table.insert(links, '<a href="' .. html(pkg.url) .. '">' .. html(pkg.title) .. "</a>")
  end
  table.insert(links, '<a href="' .. repo .. '">GitHub</a>')

  local raw = function(s) return pandoc.MetaBlocks({ pandoc.RawBlock("html", s) }) end
  doc.meta.outdir = nil
  doc.meta.site = site
  doc.meta.pagetitle = (page == "index.html" or not h1) and site or h1 .. " - " .. site
  doc.meta.description = description or site
  doc.meta.nav = raw('<ul><li><a href="index.html"' .. (page == "index.html" and ' class="on" aria-current="page"' or "") .. ">Overview</a></li>" .. table.concat(items) .. "</ul>")
  doc.meta.links = raw(table.concat(links))
  doc.meta.source = repo .. "/blob/main/" .. src
  doc.meta.srcname = src
  doc.meta.home = page == "index.html" and "true" or nil
  return doc
end
