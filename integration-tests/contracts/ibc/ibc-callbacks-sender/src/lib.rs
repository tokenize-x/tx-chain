#[cfg(not(feature = "library"))]
use cosmwasm_std::entry_point;
use cosmwasm_schema::cw_serde;
use cosmwasm_std::{
    BankMsg, Coin, DepsMut, Env, IbcBasicResponse, IbcSourceCallbackMsg, IbcSrcCallback,
    MessageInfo, Response, StdResult, TransferMsgBuilder,
};
use cw_storage_plus::Item;

#[cw_serde]
pub struct InstantiateMsg {
    /// Address receiving `amount` when the source callback runs.
    pub recipient: String,
    /// Coin sent from the contract balance when the source callback runs.
    pub amount: Coin,
}

#[cw_serde]
pub enum ExecuteMsg {
    /// Sends an IBC transfer with the contract registered as the source callback.
    TransferFunds {
        channel: String,
        amount: Coin,
        recipient: String,
    },
}

const CONFIG: Item<InstantiateMsg> = Item::new("config");

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    _info: MessageInfo,
    msg: InstantiateMsg,
) -> StdResult<Response> {
    CONFIG.save(deps.storage, &msg)?;
    Ok(Response::new().add_attribute("method", "instantiate"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    _deps: DepsMut,
    env: Env,
    _info: MessageInfo,
    msg: ExecuteMsg,
) -> StdResult<Response> {
    match msg {
        ExecuteMsg::TransferFunds {
            channel,
            amount,
            recipient,
        } => {
            let msg = TransferMsgBuilder::new(
                channel,
                recipient,
                amount,
                env.block.time.plus_minutes(5),
            )
            .with_src_callback(IbcSrcCallback {
                address: env.contract.address,
                gas_limit: None,
            })
            .build();
            Ok(Response::new()
                .add_message(msg)
                .add_attribute("action", "transfer_funds"))
        }
    }
}

/// Sends the configured coin on both the acknowledgement and the timeout of the transfer.
#[cfg_attr(not(feature = "library"), entry_point)]
pub fn ibc_source_callback(
    deps: DepsMut,
    _env: Env,
    _msg: IbcSourceCallbackMsg,
) -> StdResult<IbcBasicResponse> {
    let config = CONFIG.load(deps.storage)?;
    Ok(IbcBasicResponse::new()
        .add_message(BankMsg::Send {
            to_address: config.recipient,
            amount: vec![config.amount],
        })
        .add_attribute("action", "ibc_source_callback"))
}
