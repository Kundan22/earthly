package earthfile2llb

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/vladaionescu/earthly/earthfile2llb/parser"
)

var _ parser.EarthParserListener = &listener{}

type listener struct {
	*parser.BaseEarthParserListener
	converter *Converter
	ctx       context.Context

	executeTarget   string
	currentTarget   string
	saveImageExists bool

	imageName      string
	saveImageNames []string
	artifactName   string
	asName         string
	copySrcs       []string
	copyDest       string
	fullTargetName string
	runArgs        []string
	entrypointArgs []string
	saveFrom       string
	saveTo         string
	saveAsLocalTo  string
	workdirPath    string
	envArgKey      string
	envArgValue    string
	gitURL         string
	flagKeyValues  []string

	err error
}

func newListener(ctx context.Context, converter *Converter, executeTarget string) *listener {
	return &listener{
		ctx:           ctx,
		converter:     converter,
		executeTarget: executeTarget,
		currentTarget: "base",
	}
}

func (l *listener) EnterTargetHeader(c *parser.TargetHeaderContext) {
	// Apply implicit SAVE IMAGE for +base.
	if l.executeTarget == "base" {
		if !l.saveImageExists {
			l.converter.SaveImage(l.ctx, []string{})
		}
		l.saveImageExists = true
	}

	l.currentTarget = strings.TrimSuffix(c.GetText(), ":")
	if l.shouldSkip() {
		return
	}
	if l.currentTarget == "base" {
		l.err = errors.New("Target name cannot be base")
		return
	}
	// Apply implicit FROM +base
	err := l.converter.From(l.ctx, "+base", nil)
	if err != nil {
		l.err = errors.Wrap(err, "apply implicit FROM +base")
		return
	}
}

//
// Commands.

func (l *listener) EnterFromStmt(c *parser.FromStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.flagKeyValues = nil
	l.imageName = ""
	l.asName = ""
}

func (l *listener) ExitFromStmt(c *parser.FromStmtContext) {
	if l.shouldSkip() {
		return
	}
	buildArgs, err := parseBuildArgFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse build arg flags")
		return
	}
	err = l.converter.From(l.ctx, l.imageName, buildArgs)
	if err != nil {
		l.err = errors.Wrapf(err, "apply FROM %s", l.imageName)
		return
	}
}

func (l *listener) EnterCopyArgsArtifact(c *parser.CopyArgsArtifactContext) {
	if l.shouldSkip() {
		return
	}
	l.artifactName = ""
	l.copyDest = ""
	l.flagKeyValues = nil
}

func (l *listener) ExitCopyArgsArtifact(c *parser.CopyArgsArtifactContext) {
	if l.shouldSkip() {
		return
	}
	buildArgs, err := parseBuildArgFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse build arg flags")
		return
	}
	err = l.converter.CopyArtifact(l.ctx, l.artifactName, l.copyDest, buildArgs)
	if err != nil {
		l.err = errors.Wrapf(err, "apply COPY --artifact %s %s", l.artifactName, l.copyDest)
		return
	}
}

func (l *listener) EnterCopyArgsClassical(c *parser.CopyArgsClassicalContext) {
	if l.shouldSkip() {
		return
	}
	l.copySrcs = nil
	l.copyDest = ""
}

func (l *listener) ExitCopyArgsClassical(c *parser.CopyArgsClassicalContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.CopyClassical(l.ctx, l.copySrcs, l.copyDest)
}

func (l *listener) EnterRunStmt(c *parser.RunStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.runArgs = nil
	l.flagKeyValues = nil
}

func (l *listener) ExitRunStmt(c *parser.RunStmtContext) {
	if l.shouldSkip() {
		return
	}
	restArgs, mounts, secretKeyValues, privileged, withEntrypoint, withDocker, err := parseRunFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse run flags")
		return
	}
	args := append(restArgs, l.runArgs...)
	err = l.converter.Run(l.ctx, args, mounts, secretKeyValues, privileged, withEntrypoint, withDocker)
	if err != nil {
		l.err = errors.Wrap(err, "run")
		return
	}
}

func (l *listener) EnterSaveArtifact(c *parser.SaveArtifactContext) {
	if l.shouldSkip() {
		return
	}
	l.saveFrom = ""
	l.saveTo = ""
	l.saveAsLocalTo = ""
}

func (l *listener) ExitSaveArtifact(c *parser.SaveArtifactContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.SaveArtifact(l.ctx, l.saveFrom, l.saveTo, l.saveAsLocalTo)
}

func (l *listener) EnterSaveImage(c *parser.SaveImageContext) {
	if l.shouldSkip() {
		return
	}
	if l.saveImageExists {
		l.err = fmt.Errorf(
			"More than one SAVE IMAGE statement per target not allowed: %s", c.GetText())
		return
	}
	l.saveImageExists = true

	l.saveImageNames = nil
}

func (l *listener) ExitSaveImage(c *parser.SaveImageContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.SaveImage(l.ctx, l.saveImageNames)
}

func (l *listener) EnterBuildStmt(c *parser.BuildStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.fullTargetName = ""
	l.flagKeyValues = nil
}

func (l *listener) ExitBuildStmt(c *parser.BuildStmtContext) {
	if l.shouldSkip() {
		return
	}
	buildArgs, err := parseBuildArgFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse build arg flags")
		return
	}
	_, err = l.converter.Build(l.ctx, l.fullTargetName, buildArgs)
	if err != nil {
		l.err = errors.Wrapf(err, "apply BUILD %s", l.fullTargetName)
		return
	}
}

func (l *listener) EnterWorkdirStmt(c *parser.WorkdirStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.workdirPath = ""
}

func (l *listener) ExitWorkdirStmt(c *parser.WorkdirStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.Workdir(l.ctx, l.workdirPath)
}

func (l *listener) EnterEntrypointStmt(c *parser.EntrypointStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.entrypointArgs = nil
}

func (l *listener) ExitEntrypointStmt(c *parser.EntrypointStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.Entrypoint(l.ctx, l.entrypointArgs)
}

func (l *listener) EnterEnvStmt(c *parser.EnvStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.envArgKey = ""
	l.envArgValue = ""
}

func (l *listener) ExitEnvStmt(c *parser.EnvStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.Env(l.ctx, l.envArgKey, l.envArgValue)
}

func (l *listener) EnterArgStmt(c *parser.ArgStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.envArgKey = ""
	l.envArgValue = ""
}

func (l *listener) ExitArgStmt(c *parser.ArgStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.converter.Arg(l.ctx, l.envArgKey, l.envArgValue)
}

func (l *listener) EnterGitCloneStmt(c *parser.GitCloneStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.gitURL = ""
	l.flagKeyValues = nil
	l.copyDest = ""
}

func (l *listener) ExitGitCloneStmt(c *parser.GitCloneStmtContext) {
	if l.shouldSkip() {
		return
	}
	branch, err := parseGitCloneFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse git clone flags")
		return
	}
	err = l.converter.GitClone(l.ctx, l.gitURL, branch, l.copyDest)
	if err != nil {
		l.err = errors.Wrap(err, "git clone")
		return
	}
}

func (l *listener) EnterDockerLoadStmt(c *parser.DockerLoadStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.fullTargetName = ""
	l.imageName = ""
	l.flagKeyValues = nil
}

func (l *listener) ExitDockerLoadStmt(c *parser.DockerLoadStmtContext) {
	if l.shouldSkip() {
		return
	}
	buildArgs, err := parseBuildArgFlags(l.flagKeyValues)
	if err != nil {
		l.err = errors.Wrap(err, "parse build arg flags")
		return
	}
	err = l.converter.DockerLoad(l.ctx, l.fullTargetName, l.imageName, buildArgs)
	if err != nil {
		l.err = errors.Wrap(err, "docker load")
		return
	}
}

func (l *listener) EnterDockerPullStmt(c *parser.DockerPullStmtContext) {
	if l.shouldSkip() {
		return
	}
	l.imageName = ""
}

func (l *listener) ExitDockerPullStmt(c *parser.DockerPullStmtContext) {
	if l.shouldSkip() {
		return
	}
	err := l.converter.DockerPull(l.ctx, l.imageName)
	if err != nil {
		l.err = errors.Wrap(err, "docker pull")
		return
	}
}

//
// Variables.

func (l *listener) EnterImageName(c *parser.ImageNameContext) {
	if l.shouldSkip() {
		return
	}
	l.imageName = c.GetText()
}

func (l *listener) EnterSaveImageName(c *parser.SaveImageNameContext) {
	if l.shouldSkip() {
		return
	}
	l.saveImageNames = append(l.saveImageNames, c.GetText())
}

func (l *listener) EnterAsName(c *parser.AsNameContext) {
	if l.shouldSkip() {
		return
	}
	l.asName = c.GetText()
}

func (l *listener) EnterCopySrc(c *parser.CopySrcContext) {
	if l.shouldSkip() {
		return
	}
	l.copySrcs = append(l.copySrcs, c.GetText())
}

func (l *listener) EnterCopyDest(c *parser.CopyDestContext) {
	if l.shouldSkip() {
		return
	}
	l.copyDest = c.GetText()
}

func (l *listener) EnterRunArg(c *parser.RunArgContext) {
	if l.shouldSkip() {
		return
	}
	l.runArgs = append(l.runArgs, c.GetText())
}

func (l *listener) EnterEntrypointArg(c *parser.EntrypointArgContext) {
	if l.shouldSkip() {
		return
	}
	l.entrypointArgs = append(l.entrypointArgs, c.GetText())
}

func (l *listener) EnterSaveFrom(c *parser.SaveFromContext) {
	if l.shouldSkip() {
		return
	}
	l.saveFrom = c.GetText()
}

func (l *listener) EnterSaveTo(c *parser.SaveToContext) {
	if l.shouldSkip() {
		return
	}
	l.saveTo = c.GetText()
}

func (l *listener) EnterSaveAsLocalTo(c *parser.SaveAsLocalToContext) {
	if l.shouldSkip() {
		return
	}
	l.saveAsLocalTo = c.GetText()
}

func (l *listener) EnterFullTargetName(c *parser.FullTargetNameContext) {
	if l.shouldSkip() {
		return
	}
	l.fullTargetName = c.GetText()
}

func (l *listener) EnterArtifactName(c *parser.ArtifactNameContext) {
	if l.shouldSkip() {
		return
	}
	l.artifactName = c.GetText()
}

func (l *listener) EnterWorkdirPath(c *parser.WorkdirPathContext) {
	if l.shouldSkip() {
		return
	}
	l.workdirPath = c.GetText()
}

func (l *listener) EnterEnvArgKey(c *parser.EnvArgKeyContext) {
	if l.shouldSkip() {
		return
	}
	l.envArgKey = c.GetText()
}

func (l *listener) EnterEnvArgValue(c *parser.EnvArgValueContext) {
	if l.shouldSkip() {
		return
	}
	l.envArgValue = c.GetText()
}

func (l *listener) EnterFlagKeyValue(c *parser.FlagKeyValueContext) {
	if l.shouldSkip() {
		return
	}
	l.flagKeyValues = append(l.flagKeyValues, c.GetText())
}

func (l *listener) EnterFlagKey(c *parser.FlagKeyContext) {
	if l.shouldSkip() {
		return
	}
	l.flagKeyValues = append(l.flagKeyValues, c.GetText())
}

func (l *listener) EnterGitURL(c *parser.GitURLContext) {
	if l.shouldSkip() {
		return
	}
	l.gitURL = c.GetText()
}

func (l *listener) shouldSkip() bool {
	return l.err != nil || l.currentTarget != l.executeTarget
}

func parseBuildArgFlags(flagKeyValues []string) ([]string, error) {
	var out []string
	for _, flag := range flagKeyValues {
		split := strings.SplitN(flag, "=", 2)
		if len(split) != 2 {
			return nil, fmt.Errorf("Invalid flag format %s", flag)
		}
		if split[0] != "--build-arg" {
			return nil, fmt.Errorf("Invalid flag %s", split[0])
		}
		out = append(out, split[1])
	}
	return out, nil
}

func parseRunFlags(flagKeyValues []string) ([]string, []string, []string, bool, bool, bool, error) {
	// TODO: Clean up return values.
	// TODO: Use a flags parser.
	var restArgs []string
	var mounts []string
	var secrets []string
	privileged := false
	entrypoint := false
	withDocker := false
	for index, flag := range flagKeyValues {
		split := strings.SplitN(flag, "=", 2)
		if len(split) < 1 {
			return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
		}
		switch split[0] {
		case "--secret":
			if len(split) != 2 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			secrets = append(secrets, split[1])
		case "--privileged":
			if len(split) != 1 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			privileged = true
		case "--entrypoint":
			if len(split) != 1 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			entrypoint = true
		case "--mount":
			if len(split) != 2 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			mounts = append(mounts, split[1])
		case "--with-docker":
			if len(split) != 1 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			privileged = true
			withDocker = true
		case "--":
			if len(split) != 1 {
				return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag format %s", flag)
			}
			// The rest are regular run args.
			if index+1 < len(flagKeyValues) {
				restArgs = flagKeyValues[(index + 1):]
				return restArgs, mounts, secrets, privileged, entrypoint, withDocker, nil
			}
		default:
			return nil, nil, nil, false, false, false, fmt.Errorf("Invalid flag %s", split[0])
		}
	}
	return restArgs, mounts, secrets, privileged, entrypoint, withDocker, nil
}

func parseGitCloneFlags(flagKeyValues []string) (string, error) {
	branch := ""
	for _, flag := range flagKeyValues {
		split := strings.SplitN(flag, "=", 2)
		if len(split) != 2 {
			return "", fmt.Errorf("Invalid flag format %s", flag)
		}
		if split[0] != "--branch" {
			return "", fmt.Errorf("Invalid flag %s", split[0])
		}
		branch = split[1]
	}
	return branch, nil
}