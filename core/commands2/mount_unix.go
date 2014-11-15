// +build linux darwin freebsd

package commands

import (
	"fmt"
	"strings"
	"time"

	cmds "github.com/jbenet/go-ipfs/commands"
	config "github.com/jbenet/go-ipfs/config"
	core "github.com/jbenet/go-ipfs/core"
	ipns "github.com/jbenet/go-ipfs/fuse/ipns"
	rofs "github.com/jbenet/go-ipfs/fuse/readonly"
)

// amount of time to wait for mount errors
// TODO is this non-deterministic?
const mountTimeout = time.Second

// fuseNoDirectory used to check the returning fuse error
const fuseNoDirectory = "fusermount: failed to access mountpoint"

var mountCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Mounts IPFS to the filesystem (read-only)",
		ShortDescription: `
Mount ipfs at a read-only mountpoint on the OS (default: /ipfs and /ipns).
All ipfs objects will be accessible under that directory. Note that the
root will not be listable, as it is virtual. Access known paths directly.

You may have to create /ipfs and /ipfs before using 'ipfs mount':

> sudo mkdir /ipfs /ipns
> sudo chown ` + "`" + `whoami` + "`" + ` /ipfs /ipns
> ipfs mount
`,
		LongDescription: `
Mount ipfs at a read-only mountpoint on the OS (default: /ipfs and /ipns).
All ipfs objects will be accessible under that directory. Note that the
root will not be listable, as it is virtual. Access known paths directly.

You may have to create /ipfs and /ipfs before using 'ipfs mount':

> sudo mkdir /ipfs /ipns
> sudo chown ` + "`" + `whoami` + "`" + ` /ipfs /ipns
> ipfs mount

EXAMPLE:

# setup
> mkdir foo
> echo "baz" > foo/bar
> ipfs add -r foo
added QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR foo/bar
added QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC foo
> ipfs ls QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR 12 bar
> ipfs cat QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz

# mount
> ipfs daemon &
> ipfs mount
IPFS mounted at: /ipfs
IPNS mounted at: /ipns
> cd /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
> ls
bar
> cat bar
baz
> cat /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC/bar
baz
> cat /ipfs/QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz
`,
	},

	Options: []cmds.Option{
		// TODO longform
		cmds.StringOption("f", "The path where IPFS should be mounted"),

		// TODO longform
		cmds.StringOption("n", "The path where IPNS should be mounted"),
	},
	Run: func(req cmds.Request) (interface{}, error) {
		cfg, err := req.Context().GetConfig()
		if err != nil {
			return nil, err
		}

		node, err := req.Context().GetNode()
		if err != nil {
			return nil, err
		}

		// error if we aren't running node in online mode
		if node.Network == nil {
			return nil, errNotOnline
		}

		if err := platformFuseChecks(); err != nil {
			return nil, err
		}

		fsdir, found, err := req.Option("f").String()
		if err != nil {
			return nil, err
		}
		if !found {
			fsdir = cfg.Mounts.IPFS // use default value
		}
		fsdone := mountIpfs(node, fsdir)

		// get default mount points
		nsdir, found, err := req.Option("n").String()
		if err != nil {
			return nil, err
		}
		if !found {
			nsdir = cfg.Mounts.IPNS // NB: be sure to not redeclare!
		}

		nsdone := mountIpns(node, nsdir, fsdir)

		fmtFuseErr := func(err error) error {
			s := err.Error()
			if strings.Contains(s, fuseNoDirectory) {
				s = strings.Replace(s, `fusermount: "fusermount:`, "", -1)
				s = strings.Replace(s, `\n", exit status 1`, "", -1)
				return cmds.ClientError(s)
			}
			return err
		}

		// wait until mounts return an error (or timeout if successful)
		select {
		case err := <-fsdone:
			return nil, fmtFuseErr(err)
		case err := <-nsdone:
			return nil, fmtFuseErr(err)

		// mounted successfully, we timed out with no errors
		case <-time.After(mountTimeout):
			output := cfg.Mounts
			return &output, nil
		}
	},
	Type: &config.Mounts{},
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) ([]byte, error) {
			v := res.Output().(*config.Mounts)
			s := fmt.Sprintf("IPFS mounted at: %s\n", v.IPFS)
			s += fmt.Sprintf("IPNS mounted at: %s\n", v.IPNS)
			return []byte(s), nil
		},
	},
}

func mountIpfs(node *core.IpfsNode, fsdir string) <-chan error {
	done := make(chan error)
	log.Info("Mounting IPFS at ", fsdir)

	go func() {
		err := rofs.Mount(node, fsdir)
		done <- err
		close(done)
	}()

	return done
}

func mountIpns(node *core.IpfsNode, nsdir, fsdir string) <-chan error {
	if nsdir == "" {
		return nil
	}
	done := make(chan error)
	log.Info("Mounting IPNS at ", nsdir)

	go func() {
		err := ipns.Mount(node, nsdir, fsdir)
		done <- err
		close(done)
	}()

	return done
}

var platformFuseChecks = func() error {
	return nil
}