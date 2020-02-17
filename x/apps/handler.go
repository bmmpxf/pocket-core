package pos

import (
	"fmt"
	"github.com/pokt-network/pocket-core/x/apps/keeper"
	"github.com/pokt-network/pocket-core/x/apps/types"
	sdk "github.com/pokt-network/posmint/types"
)

func NewHandler(k keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		ctx = ctx.WithEventManager(sdk.NewEventManager())
		switch msg := msg.(type) {
		case types.MsgAppStake:
			return handleStake(ctx, msg, k)
		case types.MsgBeginAppUnstake:
			return handleMsgBeginUnstake(ctx, msg, k)
		case types.MsgAppUnjail:
			return handleMsgUnjail(ctx, msg, k)
		default:
			errMsg := fmt.Sprintf("unrecognized staking message type: %T", msg)
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

func handleStake(ctx sdk.Context, msg types.MsgAppStake, k keeper.Keeper) sdk.Result {
	// create application object using the message fields
	application := types.NewApplication(sdk.Address(msg.PubKey.Address()), msg.PubKey, msg.Chains, msg.Value)
	// check if they can stake
	if err := k.ValidateApplicationStaking(ctx, application, msg.Value); err != nil {
		return err.Result()
	}
	// change the application state to staked
	err := k.StakeApplication(ctx, application, msg.Value)
	if err != nil {
		return err.Result()
	}
	// create the event
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeCreateApplication,
			sdk.NewAttribute(types.AttributeKeyApplication, sdk.Address(msg.PubKey.Address()).String()),
		),
		sdk.NewEvent(
			types.EventTypeStake,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, sdk.Address(msg.PubKey.Address()).String()),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Value.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, sdk.Address(msg.PubKey.Address()).String()),
		),
	})
	return sdk.Result{Events: ctx.EventManager().Events()}
}

func handleMsgBeginUnstake(ctx sdk.Context, msg types.MsgBeginAppUnstake, k keeper.Keeper) sdk.Result {
	// move coins from the msg.Address account to a (self-delegation) delegator account
	// the application account and global shares are updated within here
	application, found := k.GetApplication(ctx, msg.Address)
	if !found {
		return types.ErrNoApplicationFound(k.Codespace()).Result()
	}
	if err := k.ValidateApplicationBeginUnstaking(ctx, application); err != nil {
		return err.Result()
	}
	if err := k.BeginUnstakingApplication(ctx, application); err != nil {
		return err.Result()
	}
	// create the event
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeBeginUnstake,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.Address.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.Address.String()),
		),
	})
	return sdk.Result{Events: ctx.EventManager().Events()}
}

// Applications must submit a transaction to unjail itself after todo
// having been jailed (and thus unstaked) for downtime
func handleMsgUnjail(ctx sdk.Context, msg types.MsgAppUnjail, k keeper.Keeper) sdk.Result {
	consAddr, err := validateUnjailMessage(ctx, msg, k)
	if err != nil {
		return err.Result()
	}
	k.UnjailApplication(ctx, consAddr)
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
			sdk.NewAttribute(sdk.AttributeKeySender, msg.AppAddr.String()),
		),
	)
	return sdk.Result{Events: ctx.EventManager().Events()}
}

func validateUnjailMessage(ctx sdk.Context, msg types.MsgAppUnjail, k keeper.Keeper) (consAddr sdk.Address, err sdk.Error) {
	application := k.Application(ctx, msg.AppAddr)
	if application == nil {
		return nil, types.ErrNoApplicationForAddress(k.Codespace())
	}
	// cannot be unjailed if no self-delegation exists
	selfDel := application.GetTokens()
	if selfDel == sdk.ZeroInt() {
		return nil, types.ErrMissingAppStake(k.Codespace())
	}
	if application.GetTokens().LT(sdk.NewInt(k.MinimumStake(ctx))) {
		return nil, types.ErrStakeTooLow(k.Codespace())
	}
	// cannot be unjailed if not jailed
	if !application.IsJailed() {
		return nil, types.ErrApplicationNotJailed(k.Codespace())
	}
	return
}
