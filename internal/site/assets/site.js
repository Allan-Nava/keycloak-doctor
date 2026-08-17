// The only script on the site: filtering 30 rules by hand is worse than 20 lines
// of JavaScript, and the page works without it — the rules are plain HTML.
(function () {
  "use strict";

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
      var visible = section.querySelectorAll(".rule:not([hidden])").length;
      section.hidden = visible === 0;
    });

    if (count) {
      count.textContent = shown === total ? total + " rules" : shown + " of " + total + " rules";
    }
  }

  input.addEventListener("input", apply);
  input.removeAttribute("hidden");
  apply();
})();
