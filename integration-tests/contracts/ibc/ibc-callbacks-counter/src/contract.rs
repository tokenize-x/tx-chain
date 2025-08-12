use crate::error::ContractError;
use crate::msg::*;
use crate::state::{Counter, COUNTERS};
#[cfg(not(feature = "library"))]
use cosmwasm_std::entry_point;
use cosmwasm_std::{
    coins, ensure_eq, from_json, to_json_binary, Addr, BankMsg, Binary, Coin, Deps, DepsMut, Env,
    IbcAckCallbackMsg, IbcBasicResponse, IbcDestinationCallbackMsg, IbcDstCallback,
    IbcSourceCallbackMsg, IbcSrcCallback, IbcTimeoutCallbackMsg, MessageInfo, Response, StdAck,
    StdError, StdResult, Timestamp, TransferMsgBuilder, Uint128,
};
use cw2::set_contract_version;
use std::collections::HashMap;
use std::str::FromStr;

// version info for migration info
const CONTRACT_NAME: &str = "callbacks_counter";
const CONTRACT_VERSION: &str = env!("CARGO_PKG_VERSION");

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    set_contract_version(deps.storage, CONTRACT_NAME, CONTRACT_VERSION)?;
    let initial_counter = Counter {
        count: msg.count,
        total_funds: vec![],
        owner: info.sender.clone(),
    };
    COUNTERS.save(deps.storage, info.sender.clone(), &initial_counter)?;

    Ok(Response::new()
        .add_attribute("method", "instantiate")
        .add_attribute("owner", info.sender)
        .add_attribute("count", msg.count.to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    _deps: DepsMut,
    env: Env,
    _info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::TransferFunds {
            channel,
            amount,
            recipient,
        } => transfer_funds(env, channel, amount, recipient),
    }
}

pub fn transfer_funds(
    env: Env,
    channel: String,
    amount: Coin,
    recipient: String,
) -> Result<Response, ContractError> {
    let msg = TransferMsgBuilder::new(
        channel.to_string(),
        recipient.to_string(),
        amount.clone(),
        env.block.time.plus_minutes(5),
    )
    .with_src_callback(IbcSrcCallback {
        address: env.contract.address,
        gas_limit: None,
    })
    .build();

    Ok(Response::new()
        .add_message(msg)
        .add_attribute("action", "transfer_funds")
        .add_attribute("channel", channel)
        .add_attribute("amount", amount.to_string())
        .add_attribute("recipient", recipient))
}

pub mod utils {
    use cosmwasm_std::Addr;

    use super::*;

    pub fn update_counter(
        deps: DepsMut,
        sender: Addr,
        update_counter: &dyn Fn(&Option<Counter>) -> i32,
        update_funds: &dyn Fn(&Option<Counter>) -> Vec<Coin>,
    ) -> Result<bool, ContractError> {
        COUNTERS
            .update(
                deps.storage,
                sender.clone(),
                |state| -> Result<_, ContractError> {
                    match state {
                        None => Ok(Counter {
                            count: update_counter(&None),
                            total_funds: update_funds(&None),
                            owner: sender,
                        }),
                        Some(counter) => Ok(Counter {
                            count: update_counter(&Some(counter.clone())),
                            total_funds: update_funds(&Some(counter)),
                            owner: sender,
                        }),
                    }
                },
            )
            .map(|_r| true)
    }
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn ibc_source_callback(
    deps: DepsMut,
    env: Env,
    msg: IbcSourceCallbackMsg,
) -> StdResult<IbcBasicResponse> {
    match msg {
        IbcSourceCallbackMsg::Acknowledgement(IbcAckCallbackMsg { .. }) => {
            receive_ack(deps, env.contract.address.clone(), true)
                .map_err(|e| StdError::generic_err(e.to_string()))?;
        }
        IbcSourceCallbackMsg::Timeout(IbcTimeoutCallbackMsg { .. }) => {
            ibc_timeout(deps, env.contract.address.clone())
                .map_err(|e| StdError::generic_err(e.to_string()))?;
        }
    }

    Ok(IbcBasicResponse::new().add_attribute("action", "ibc_source_callback"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn ibc_destination_callback(
    deps: DepsMut,
    env: Env,
    msg: IbcDestinationCallbackMsg,
) -> StdResult<IbcBasicResponse> {
    ensure_eq!(
        msg.packet.dest.port_id,
        "transfer", // transfer module uses this port by default
        StdError::generic_err("only want to handle transfer packets")
    );
    ensure_eq!(
        msg.ack.data,
        StdAck::success(b"\x01").to_binary(), // this is how a successful transfer ack looks
        StdError::generic_err("only want to handle successful transfers")
    );

    receive_ack(deps, env.contract.address.clone(), true)
        .map_err(|e| StdError::generic_err(e.to_string()))?;

    Ok(IbcBasicResponse::new().add_attribute("action", "ibc_destination_callback"))
}

pub fn receive_ack(
    deps: DepsMut,
    contract: Addr,
    _success: bool,
) -> Result<Response, ContractError> {
    utils::update_counter(
        deps,
        contract,
        &|counter| match counter {
            None => 1,
            Some(counter) => counter.count + 1,
        },
        &|_counter| vec![],
    )?;
    Ok(Response::new().add_attribute("action", "ack"))
}

pub fn ibc_timeout(deps: DepsMut, contract: Addr) -> Result<Response, ContractError> {
    utils::update_counter(
        deps,
        contract,
        &|counter| match counter {
            None => 10,
            Some(counter) => counter.count + 10,
        },
        &|_counter| vec![],
    )?;
    Ok(Response::new().add_attribute("action", "timeout"))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::GetCount { addr } => to_json_binary(&query::count(deps, addr)?),
        QueryMsg::GetTotalFunds { addr } => to_json_binary(&query::total_funds(deps, addr)?),
    }
}

pub mod query {
    use cosmwasm_std::Addr;

    use super::*;

    pub fn count(deps: Deps, addr: Addr) -> StdResult<GetCountResponse> {
        let state = COUNTERS.load(deps.storage, addr)?;
        Ok(GetCountResponse { count: state.count })
    }

    pub fn total_funds(deps: Deps, addr: Addr) -> StdResult<GetTotalFundsResponse> {
        let state = COUNTERS.load(deps.storage, addr)?;
        Ok(GetTotalFundsResponse {
            total_funds: state.total_funds,
        })
    }
}
