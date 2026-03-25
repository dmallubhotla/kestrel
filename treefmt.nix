# treefmt.nix
{ ... }:
{
  projectRootFile = "treefmt.nix";
  settings.global.excludes = [
    "examples/*"
  ];

  programs.deadnix.enable = true;
  programs.mdsh.enable = true;
  programs.nixfmt.enable = true;
  programs.shellcheck.enable = true;
  settings.formatter.shellcheck = {
    excludes = [ "*.envrc*" ];
  };

  programs.shfmt.enable = true;
  settings.formatter.shfmt.indent_size = 0;

  programs.yamlfmt.enable = true;
  programs.just.enable = true;

  programs.gofmt.enable = true;
}
