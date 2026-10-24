{
  description = "An application to control your Elgato Stream Deck on Linux";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        deckmaster = pkgs.buildGo127Module {
          pname = "deckmaster";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = nixpkgs.lib.cleanSource ./.;

          # Update when go.mod/go.sum change: set to nixpkgs.lib.fakeHash, build, copy the hash.
          vendorHash = "sha256-zy6pXeQABN7FBV2aTWkGSuReFrDIiJtKK+69rmSYpvc=";

          ldflags = [
            "-s"
            "-w"
            "-X main.CommitSHA=${self.rev or self.dirtyRev or ""}"
          ];

          nativeBuildInputs = [ pkgs.makeWrapper ];

          postInstall = ''
            install -Dm644 udev/70-streamdeck.rules $out/lib/udev/rules.d/70-streamdeck.rules
            # deckmaster looks up its default fonts (Roboto) via XDG_DATA_DIRS,
            # and pastes on Wayland with wl-copy
            wrapProgram $out/bin/deckmaster \
              --suffix XDG_DATA_DIRS : ${pkgs.roboto}/share \
              --suffix PATH : ${pkgs.wl-clipboard}/bin
          '';

          meta = {
            description = "An application to control your Elgato Stream Deck on Linux";
            homepage = "https://github.com/nix-er/deckmaster";
            license = nixpkgs.lib.licenses.mit;
            mainProgram = "deckmaster";
            platforms = nixpkgs.lib.platforms.linux;
          };
        };
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;
      });

      overlays.default = final: _prev: {
        deckmaster = self.packages.${final.stdenv.hostPlatform.system}.deckmaster;
      };

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_27
            gcc
            pkg-config
            golangci-lint
            goreleaser
            govulncheck
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
