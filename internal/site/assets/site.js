// The only script on the site, and nothing here is load-bearing: the rules are
// plain HTML, the table of contents is a list of links, and a code block is
// selectable text. This adds the three affordances a reference page wants —
// filtering, knowing where you are, and copying a command — and gets out of the
// way when the browser cannot do them.
(function () {
  "use strict";

  // Each affordance is independent: one that throws on an old browser must not
  // take the other two down with it.
  [filterRules, highlightCurrentSection, addCopyButtons].forEach(function (feature) {
    try {
      feature();
    } catch (err) {
      if (window.console) window.console.warn("site.js: " + feature.name + " is off:", err);
    }
  });

  // Filtering 30 rules by hand is worse than twenty lines of JavaScript.
  function filterRules() {
    var input = document.querySelector("#rule-filter");
    if (!input) return;

    var count = document.querySelector("#rule-count");
    var rules = Array.prototype.slice.call(document.querySelectorAll(".rule"));
    var sections = Array.prototype.slice.call(document.querySelectorAll(".category"));
    var total = rules.length;

    function apply() {
      var q = input.value.trim().toLowerCase();
      var shown = 0;

      rules.forEach(function (rule) {
        var match = q === "" || rule.dataset.search.indexOf(q) !== -1;
        rule.hidden = !match;
        if (match) shown++;
      });

      // A category with nothing left in it is noise, not a heading.
      sections.forEach(function (section) {
        section.hidden = section.querySelectorAll(".rule:not([hidden])").length === 0;
      });

      if (count) {
        count.textContent = shown === total ? total + " rules" : shown + " of " + total + " rules";
      }
    }

    input.addEventListener("input", apply);
    input.removeAttribute("hidden");
    apply();
  }

  // Mark the section being read in the table of contents.
  function highlightCurrentSection() {
    var links = Array.prototype.slice.call(document.querySelectorAll(".toc a"));
    if (!links.length || !("IntersectionObserver" in window)) return;

    var byID = {};
    var targets = [];
    links.forEach(function (link) {
      var id = link.getAttribute("href").slice(1);
      var heading = document.getElementById(id);
      if (!heading) return;
      byID[id] = link;
      targets.push(heading);
    });

    var visible = {};
    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          visible[entry.target.id] = entry.isIntersecting;
        });
        var current = null;
        targets.forEach(function (heading) {
          if (visible[heading.id] && !current) current = heading.id;
        });
        links.forEach(function (link) {
          link.classList.remove("active");
        });
        if (current && byID[current]) byID[current].classList.add("active");
      },
      // A band just under the sticky header: the heading you just scrolled past is
      // the section you are in. rootMargin only accepts px or %, never rem.
      { rootMargin: "-88px 0px -70% 0px" }
    );
    targets.forEach(function (heading) {
      observer.observe(heading);
    });
  }

  // Install commands are meant to be copied, not retyped.
  function addCopyButtons() {
    if (!navigator.clipboard) return;

    Array.prototype.slice.call(document.querySelectorAll("main pre")).forEach(function (pre) {
      var wrap = document.createElement("div");
      wrap.className = "code-block";
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);

      var button = document.createElement("button");
      button.type = "button";
      button.className = "copy";
      button.textContent = "Copy";
      button.setAttribute("aria-label", "Copy this snippet");
      wrap.appendChild(button);

      button.addEventListener("click", function () {
        // A console transcript is pasted to be run: the "$ " prompts are not part
        // of the command.
        var text = pre.innerText.replace(/^\$ /gm, "");
        navigator.clipboard.writeText(text).then(
          function () {
            button.textContent = "Copied";
            button.dataset.done = "true";
            window.setTimeout(function () {
              button.textContent = "Copy";
              delete button.dataset.done;
            }, 1600);
          },
          function () {
            button.textContent = "Press ⌘C";
          }
        );
      });
    });
  }
})();
