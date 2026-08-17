# Homebrew formula for keycloak-doctor. It lives in this repository, which is the
# tap:
#
#   brew tap Allan-Nava/keycloak-doctor https://github.com/Allan-Nava/keycloak-doctor
#   brew install keycloak-doctor
#
# It cannot go to homebrew-core: that tap only accepts OSI-approved open-source
# licences, and this project is source-available under PolyForm Noncommercial.
#
# `tag` and `revision` always point at the last *published* release — the Release
# workflow moves them when a new tag is pushed, so this file is never edited by
# hand.
class KeycloakDoctor < Formula
  desc "Audit a Keycloak realm for the mistakes that actually get exploited"
  homepage "https://allan-nava.github.io/keycloak-doctor/"
  url "https://github.com/Allan-Nava/keycloak-doctor.git",
      tag:      "v0.1.6",
      revision: "1fc2f7c707d144ae927a12d43b2f30311c97e46b"
  license :cannot_represent
  head "https://github.com/Allan-Nava/keycloak-doctor.git", branch: "main"

  depends_on "go" => :build

  def install
    # The version is injected the same way the release binaries get it, so
    # `keycloak-doctor version` does not report "dev" on a brew install.
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/keycloak-doctor"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/keycloak-doctor version")

    # A realm with TLS not required is the smallest thing the audit has to catch,
    # and the exit code must stay 0: an audit that ran is a success, findings and
    # all.
    (testpath/"realm.json").write <<~JSON
      {"realm": "brew-test", "enabled": true, "sslRequired": "none"}
    JSON
    output = shell_output("#{bin}/keycloak-doctor audit #{testpath}/realm.json --no-color")
    assert_match "realm/ssl-required", output
    assert_match "worst: BAD", output
  end
end
