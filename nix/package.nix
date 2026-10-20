{
  lib,
  buildGoModule,
  git,
  version,
}:
buildGoModule {
  pname = "loadout";
  inherit version;

  src = lib.cleanSource ../.;

  vendorHash = "sha256-YqkKjHPwAI/Yj8MhbVxEGbEoUamx/z7I+Upk6Ik9oeI=";

  subPackages = [ "cmd/loadout" ];

  ldflags = [
    "-s"
    "-w"
    "-X github.com/Uncool-dudes/loadout/internal/cli.Version=${version}"
  ];

  # The repo and cli tests run real git commands (worktrees, info/exclude).
  nativeCheckInputs = [ git ];

  meta = {
    description = "Pick which Claude Code skills, settings and MCP servers load in a project";
    mainProgram = "loadout";
    platforms = lib.platforms.unix;
  };
}
