{
  description = "loadout: pick which Claude Code skills, settings and MCP servers load in a project";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        loadout = pkgs.callPackage ./nix/package.nix {
          version = self.shortRev or self.dirtyShortRev or "dev";
        };
        default = loadout;
      });

      overlays.default = final: _prev: {
        loadout = final.callPackage ./nix/package.nix {
          version = self.shortRev or self.dirtyShortRev or "dev";
        };
      };

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
